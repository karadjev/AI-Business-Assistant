package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
)

type FitmentQuery struct {
	VariantID string `json:"variant_id"`
	Make      string `json:"make"`
	Model     string `json:"model"`
	Year      int    `json:"year"`
	Engine    string `json:"engine"`
}

type FitmentVehicle struct {
	Make   string `json:"make"`
	Model  string `json:"model"`
	Year   int    `json:"year"`
	Engine string `json:"engine"`
}

func (v FitmentVehicle) normalized() (FitmentVehicle, error) {
	v.Make, v.Model, v.Engine = strings.TrimSpace(v.Make), strings.TrimSpace(v.Model), strings.TrimSpace(v.Engine)
	if v.Year < 1886 || v.Year > 2100 {
		return FitmentVehicle{}, invalid("Некорректный год автомобиля")
	}
	for _, value := range []string{v.Make, v.Model, v.Engine} {
		if !textValid(value, 1, 80) || strings.ContainsFunc(value, unicode.IsControl) {
			return FitmentVehicle{}, invalid("Укажите марку, модель и двигатель автомобиля")
		}
	}
	return v, nil
}

// Both reads require the same live evidence for the exact vehicle and variant.
func fitmentEvidenceMatch(first int) string {
	return fmt.Sprintf(`f.organization_id=v.organization_id AND f.variant_id=v.id
 AND f.make=lower($%d) AND f.model=lower($%d) AND f.engine=lower($%d)
 AND f.year_from<=$%d AND f.year_to>=$%d AND f.revoked_at IS NULL
 AND f.verified_at<=statement_timestamp() AND f.valid_until>statement_timestamp()`, first, first+1, first+2, first+3, first+3)
}

type FitmentStatus string

const (
	FitmentCompatible   FitmentStatus = "compatible"
	FitmentIncompatible FitmentStatus = "incompatible"
	FitmentUnknown      FitmentStatus = "unknown"
)

type FitmentEvidence struct {
	SourceRef  string    `json:"source_ref"`
	Compatible bool      `json:"compatible"`
	YearFrom   int       `json:"year_from"`
	YearTo     int       `json:"year_to"`
	VerifiedAt time.Time `json:"verified_at"`
	ValidUntil time.Time `json:"valid_until"`
}

type FitmentResult struct {
	Status    FitmentStatus     `json:"status"`
	Reason    string            `json:"reason"`
	VariantID string            `json:"variant_id"`
	Evidence  []FitmentEvidence `json:"evidence"`
}

func (s CustomerCatalog) CheckFitment(ctx context.Context, t Tenant, q FitmentQuery) (FitmentResult, error) {
	return s.service.CheckFitment(ctx, t, q)
}

func (s Service) CheckFitment(ctx context.Context, t Tenant, q FitmentQuery) (FitmentResult, error) {
	if !organizations.ValidID(q.VariantID) {
		return FitmentResult{}, invalid("Некорректный вариант")
	}
	vehicle, err := (FitmentVehicle{Make: q.Make, Model: q.Model, Year: q.Year, Engine: q.Engine}).normalized()
	if err != nil {
		return FitmentResult{}, err
	}
	tx, err := s.beginIndustry(ctx, t, "auto_parts")
	if err != nil {
		return FitmentResult{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL jit=off"); err != nil {
		return FitmentResult{}, dbError(err)
	}
	// Retrieve one verified record per polarity. Disagreement always stays
	// unknown, regardless of the number or recency of records on either side.
	raw, err := object(ctx, tx, `SELECT coalesce((
  SELECT jsonb_agg(to_jsonb(match) ORDER BY match.compatible DESC)
  FROM (VALUES (true),(false)) decision(compatible)
  CROSS JOIN LATERAL (
   SELECT f.source_ref,f.compatible,f.year_from,f.year_to,f.verified_at,f.valid_until
   FROM platform.catalog_fitment_evidence f
   WHERE f.compatible=decision.compatible AND `+fitmentEvidenceMatch(3)+`
   ORDER BY f.verified_at DESC,f.id LIMIT 1
  ) match
 ), '[]'::jsonb)
 FROM platform.product_variants v JOIN platform.products p ON p.organization_id=v.organization_id AND p.id=v.product_id
 WHERE v.organization_id=$1 AND v.id=$2 AND p.status='active'
 AND (SELECT platform.industry_catalog_profile($1))='auto_parts'`,
		t.OrganizationID, q.VariantID, vehicle.Make, vehicle.Model, vehicle.Engine, vehicle.Year)
	if err != nil {
		return FitmentResult{}, err
	}
	result := FitmentResult{Status: FitmentUnknown, Reason: "no_verified_evidence", VariantID: q.VariantID}
	if err = json.Unmarshal(raw, &result.Evidence); err != nil {
		return FitmentResult{}, dbError(err)
	}
	switch len(result.Evidence) {
	case 1:
		result.Status = FitmentIncompatible
		if result.Evidence[0].Compatible {
			result.Status = FitmentCompatible
		}
		result.Reason = "verified_evidence"
	case 2:
		result.Reason = "conflicting_verified_evidence"
	}
	return result, nil
}
