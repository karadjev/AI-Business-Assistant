package servicebooking

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
)

// SaveResource is owner-only, revision checked, and cannot edit a resource with
// active allocations. Staff booking permission is not schedule administration.
func (s Service) SaveResource(ctx context.Context, t catalog.Tenant, in Resource) (Resource, error) {
	creating := in.ID == ""
	in.ID = newID(in.ID)
	if !organizations.ValidID(in.ID) || !validText(in.Name, 128) || in.Revision < 0 {
		return Resource{}, invalid()
	}
	r := bf.Resource{Reference: bf.Reference{OrganizationID: t.OrganizationID, ID: in.ID}, Kind: in.Kind, Capacity: in.Capacity}
	if r.Validate(t.OrganizationID) != nil {
		return Resource{}, invalid()
	}
	tx, err := begin(ctx, s.Pool, t)
	if err != nil {
		return Resource{}, err
	}
	defer tx.Rollback(ctx)
	if err = guard(ctx, tx, t.OrganizationID, true); err != nil {
		return Resource{}, err
	}
	if creating {
		if in.Revision != 0 {
			return Resource{}, invalid()
		}
		err = tx.QueryRow(ctx, `INSERT INTO platform.service_resources(id,organization_id,name,kind,capacity,active) VALUES($1,$2,$3,$4,$5,$6) RETURNING revision`, in.ID, t.OrganizationID, in.Name, in.Kind, in.Capacity, in.Active).Scan(&in.Revision)
	} else {
		if in.Revision < 1 {
			return Resource{}, invalid()
		}
		err = tx.QueryRow(ctx, `UPDATE platform.service_resources SET name=$3,kind=$4,capacity=$5,active=$6,revision=revision+1 WHERE organization_id=$1 AND id=$2 AND revision=$7 RETURNING revision`, t.OrganizationID, in.ID, in.Name, in.Kind, in.Capacity, in.Active, in.Revision).Scan(&in.Revision)
	}
	if err != nil {
		return Resource{}, dbError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Resource{}, dbError(err)
	}
	return in, nil
}
func validPartRule(rule *PartRule) bool {
	if rule == nil {
		return true
	}
	for _, v := range []string{rule.Make, rule.Model, rule.Engine} {
		if v != "" && (!validText(v, 80) || strings.TrimSpace(v) != v) {
			return false
		}
	}
	if rule.Year != 0 && (rule.Year < 1886 || rule.Year > 2100) {
		return false
	}
	switch rule.State {
	case "REQUIRED", "OPTIONAL":
		return rule.Quantity >= 1 && rule.Quantity <= 10 && partRuleOEM.MatchString(rule.OEM)
	case "NOT_REQUIRED", "UNKNOWN":
		return rule.OEM == "" && rule.Quantity == 0
	}
	return false
}

var partRuleOEM = regexp.MustCompile(`^[[:alnum:]_. /-]{1,80}$`)

func (s Service) SaveOffering(ctx context.Context, t catalog.Tenant, in Offering) (Offering, error) {
	if !validPartRule(in.PartRule) {
		return Offering{}, invalid()
	}
	creating := in.ID == ""
	in.ID = newID(in.ID)
	expected := in.Revision
	if creating {
		in.Revision = 1
	}
	if err := in.validate(t.OrganizationID); err != nil {
		return Offering{}, err
	}
	sort.Strings(in.ResourceIDs)
	tx, err := begin(ctx, s.Pool, t)
	if err != nil {
		return Offering{}, err
	}
	defer tx.Rollback(ctx)
	if err = guard(ctx, tx, t.OrganizationID, true); err != nil {
		return Offering{}, err
	}
	args := []any{t.OrganizationID, in.ID, in.Name, in.Status, in.PriceModel, in.PriceState, in.AmountMinor, in.Currency, in.MinMinutes, in.MaxMinutes, in.BufferBefore, in.BufferAfter, in.Timezone, in.PartRule}
	if creating {
		if expected != 0 {
			return Offering{}, invalid()
		}
		_, err = tx.Exec(ctx, `INSERT INTO platform.service_offerings(organization_id,id,name,status,price_model,price_state,amount_minor,currency,min_minutes,max_minutes,buffer_before,buffer_after,timezone,part_rule) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb)`, args...)
	} else {
		args = append(args, expected)
		err = tx.QueryRow(ctx, `UPDATE platform.service_offerings SET name=$3,status=$4,price_model=$5,price_state=$6,amount_minor=$7,currency=$8,min_minutes=$9,max_minutes=$10,buffer_before=$11,buffer_after=$12,timezone=$13,part_rule=coalesce($14::jsonb,part_rule),revision=revision+1 WHERE organization_id=$1 AND id=$2 AND revision=$15 RETURNING revision`, args...).Scan(&in.Revision)
	}
	if err != nil {
		return Offering{}, dbError(err)
	}
	// Delete only removed assignments; unchanged schedule evidence is preserved.
	if _, err = tx.Exec(ctx, `DELETE FROM platform.service_offering_resources WHERE organization_id=$1 AND offering_id=$2 AND NOT(resource_id=ANY($3::uuid[]))`, t.OrganizationID, in.ID, in.ResourceIDs); err != nil {
		return Offering{}, dbError(err)
	}
	for _, id := range in.ResourceIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO platform.service_offering_resources(organization_id,offering_id,resource_id) SELECT $1,$2,id FROM platform.service_resources WHERE organization_id=$1 AND id=$3 AND active ON CONFLICT DO NOTHING`, t.OrganizationID, in.ID, id); err != nil {
			return Offering{}, dbError(err)
		}
	}
	current, err := loadOffering(ctx, tx, t.OrganizationID, in.ID)
	if err != nil {
		return Offering{}, err
	}
	if len(current.ResourceIDs) != len(in.ResourceIDs) {
		return Offering{}, invalid()
	}
	if err = tx.Commit(ctx); err != nil {
		return Offering{}, dbError(err)
	}
	return current, nil
}

// SaveWindow adds an absolute window. Recurrence, implicit 24/7 fallback and
// private calendar synchronization are deliberately not supplied.
func (s Service) SaveWindow(ctx context.Context, t catalog.Tenant, in Window) (Window, error) {
	in.ID = newID(in.ID)
	if !organizations.ValidID(in.ID) || !organizations.ValidID(in.OfferingID) || !organizations.ValidID(in.ResourceID) {
		return Window{}, invalid()
	}
	switch in.Layer {
	case "business", "offering", "shift", "exception":
	default:
		return Window{}, invalid()
	}
	if err := ValidateZonedInterval(in.StartsAt, in.EndsAt, in.Timezone, 31*24*time.Hour); err != nil {
		return Window{}, err
	}
	tx, err := begin(ctx, s.Pool, t)
	if err != nil {
		return Window{}, err
	}
	defer tx.Rollback(ctx)
	if err = guard(ctx, tx, t.OrganizationID, true); err != nil {
		return Window{}, err
	}
	var zone string
	if err = tx.QueryRow(ctx, `SELECT timezone FROM platform.service_offerings WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, in.OfferingID).Scan(&zone); err != nil {
		return Window{}, dbError(err)
	}
	if zone != in.Timezone {
		return Window{}, invalid()
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.service_availability_windows(id,organization_id,offering_id,resource_id,layer,starts_at,ends_at,timezone) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, in.ID, t.OrganizationID, in.OfferingID, in.ResourceID, in.Layer, in.StartsAt, in.EndsAt, in.Timezone); err != nil {
		return Window{}, dbError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Window{}, dbError(err)
	}
	return in, nil
}
func (s Service) DeleteWindow(ctx context.Context, t catalog.Tenant, id string) error {
	if !organizations.ValidID(id) {
		return invalid()
	}
	tx, err := begin(ctx, s.Pool, t)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = guard(ctx, tx, t.OrganizationID, true); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM platform.service_availability_windows WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id); err != nil {
		return dbError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return dbError(err)
	}
	return nil
}
