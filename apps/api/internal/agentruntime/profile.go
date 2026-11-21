package agentruntime

import (
	"context"
	"encoding/json"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/specialists"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type BusinessProfile struct {
	Model        string   `json:"model"`
	Industry     string   `json:"industry"`
	Name         string   `json:"name,omitempty"`
	Timezone     string   `json:"timezone"`
	Revision     int      `json:"revision"`
	Capabilities []string `json:"capabilities"`
}

var industryModels = map[string]string{
	"electronics": "trade", "auto_parts": "trade", "building_materials": "trade", "furniture": "trade", "apparel": "trade",
	"vehicle_service": "service", "personal_services": "service", "real_estate": "agency", "mixed_nonfood": "hybrid",
}

func (p BusinessProfile) valid() bool {
	return p.Model != "" && industryModels[p.Industry] == p.Model && p.Revision > 0
}

func (p BusinessProfile) catalogAllowed() bool {
	return p.valid() && (p.Model == "trade" || p.Model == "hybrid")
}

func catalogAction(action string) bool {
	switch action {
	case "get_product_price", "check_inventory", "get_product_details":
		return true
	}
	return false
}

func (p BusinessProfile) dependencies() specialists.Dependencies {
	return specialists.Dependencies{
		BusinessModel: p.Model, IndustryProfiles: []string{p.Industry},
		CatalogProfileApproved: p.catalogAllowed(), BridgeCostKnown: true,
	}
}

type teamAgent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	Enabled     bool     `json:"enabled"`
}

type Team struct {
	Profile BusinessProfile `json:"profile"`
	Agents  []teamAgent     `json:"agents"`
}

func (s Service) Team(ctx context.Context, t catalog.Tenant) (Team, error) {
	tx, err := s.ownerTx(ctx, t)
	if err != nil {
		return Team{}, err
	}
	defer tx.Rollback(ctx)
	var p BusinessProfile
	err = tx.QueryRow(ctx, `SELECT business_model,industry_profile,name,timezone,business_profile_revision
 FROM platform.organizations WHERE id=$1`, t.OrganizationID).Scan(&p.Model, &p.Industry, &p.Name, &p.Timezone, &p.Revision)
	if err != nil || !p.valid() {
		return Team{}, auth.Unavailable()
	}
	p.Capabilities = []string{"completed_order_reports", "marketing_draft_approval", "team_coordination"}
	if p.catalogAllowed() {
		p.Capabilities = append(p.Capabilities, "catalog_price", "catalog_inventory", "catalog_details", "growth_review")
	}
	regs, err := specialists.Registrations(t.OrganizationID, p.dependencies())
	if err != nil {
		return Team{}, auth.Unavailable()
	}
	result := Team{Profile: p, Agents: make([]teamAgent, 0, len(regs))}
	for _, reg := range regs {
		a := reg.Agent
		allowed := make([]string, 0, 3)
		for _, permission := range a.Permissions {
			if !p.catalogAllowed() && (a.ID == "customer" || a.ID == "sales" || a.ID == "product_inventory") {
				continue
			}
			if a.ID == "customer" || a.ID == "sales" || a.ID == "product_inventory" {
				if !catalogAction(permission[len(a.ID)+1:]) {
					continue
				}
			}
			allowed = append(allowed, permission)
		}
		result.Agents = append(result.Agents, teamAgent{ID: a.ID, Name: a.Name, Description: a.Description, Permissions: allowed, Enabled: len(allowed) > 0})
	}
	return result, nil
}

func (s Service) UpdateProfile(ctx context.Context, t catalog.Tenant, model, industry string, revision int) error {
	p := BusinessProfile{Model: model, Industry: industry, Revision: revision}
	if !p.valid() {
		return auth.Failure(422, "INVALID_BUSINESS_PROFILE", "Профиль бизнеса не поддерживается")
	}
	tx, err := s.ownerTx(ctx, t)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var changed bool
	err = tx.QueryRow(ctx, "SELECT platform.agent_update_business_profile($1,$2,$3,$4)", t.OrganizationID, model, industry, revision).Scan(&changed)
	if err != nil {
		return tenancy.Error(err)
	}
	if !changed {
		return auth.Failure(409, "PROFILE_CHANGED", "Профиль изменился. Обновите страницу")
	}
	if err = tx.Commit(ctx); err != nil {
		return auth.Unavailable()
	}
	return nil
}

func (a *attempt) loadProfile(ctx context.Context) (BusinessProfile, error) {
	var raw []byte
	if err := a.service.Pool.QueryRow(ctx, "SELECT platform.agent_business_profile($1,$2)", a.job.ID, a.lease).Scan(&raw); err != nil {
		return BusinessProfile{}, err
	}
	var p BusinessProfile
	if json.Unmarshal(raw, &p) != nil || !p.valid() {
		return BusinessProfile{}, denied()
	}
	return p, nil
}
