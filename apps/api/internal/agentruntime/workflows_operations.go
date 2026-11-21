package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

// RunStaffWorkflow requires a Session from request authentication. Session
// membership and staff rights are checked again through the organization
// service. A worker lease requires RunWorkflow with a scoped read adapter.
func (s Service) RunStaffWorkflow(ctx context.Context, actor catalog.Tenant, task af.Task, in WorkflowInput, stores WorkflowStores) (WorkflowResult, error) {
	if s.Pool == nil || actor.AgentLease != "" || actor.AgentTaskID != "" ||
		actor.ChannelType != "" || actor.ChannelCapability != "" || len(actor.Session.Hash) != sha256.Size ||
		actor.OrganizationID != task.Scope.TenantID || actor.Session.User.ID != task.Scope.PrincipalID {
		return WorkflowResult{}, af.ErrDenied
	}
	actor.Session.Hash = append([]byte(nil), actor.Session.Hash...)
	reader := staffWorkflowReader{service: s, actor: actor, scope: task.Scope}
	return RunWorkflow(ctx, task, in, WorkflowDependencies{Reader: reader, Stores: stores})
}

type staffWorkflowReader struct {
	service Service
	actor   catalog.Tenant
	scope   af.Scope
}

func (r staffWorkflowReader) CheckWorkflowScope(ctx context.Context, scope af.Scope, scenario WorkflowScenario) error {
	if scope != r.scope {
		return af.ErrDenied
	}
	org, err := (organizations.Service{Pool: r.service.Pool}).Get(ctx, r.actor.Session, scope.TenantID)
	if err != nil {
		return err
	}
	if org.ID != scope.TenantID || !org.IsTest || !org.CanManageOrders {
		return af.ErrDenied
	}
	tx, err := tenancy.Begin(ctx, r.service.Pool, r.actor, true, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var active bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.conversations
	    WHERE organization_id=$1 AND id=$2 AND status='open' AND expires_at>statement_timestamp())`,
		scope.TenantID, scope.ConversationID).Scan(&active)
	if err != nil {
		return tenancy.Error(err)
	}
	if !active {
		return af.ErrDenied
	}
	if scenario == WorkflowConstruction {
		var allowed bool
		err = tx.QueryRow(ctx, "SELECT industry_profile='building_materials' FROM platform.organizations WHERE id=$1", scope.TenantID).Scan(&allowed)
		if err != nil {
			return tenancy.Error(err)
		}
		if !allowed {
			return af.ErrDenied
		}
	}
	return nil
}

func (r staffWorkflowReader) SearchIndustry(ctx context.Context, scope af.Scope, filters catalog.IndustrySearchFilters) (catalog.IndustrySearchResult, error) {
	if scope != r.scope {
		return catalog.IndustrySearchResult{}, af.ErrDenied
	}
	return (catalog.Service{Pool: r.service.Pool}).SearchIndustry(ctx, r.actor, filters)
}

func (r staffWorkflowReader) CheckFitment(ctx context.Context, scope af.Scope, query catalog.FitmentQuery) (catalog.FitmentResult, error) {
	if scope != r.scope {
		return catalog.FitmentResult{}, af.ErrDenied
	}
	return (catalog.Service{Pool: r.service.Pool}).CheckFitment(ctx, r.actor, query)
}

func (r staffWorkflowReader) SearchOfferings(ctx context.Context, scope af.Scope, query string, limit int) ([]servicebooking.Offering, error) {
	if scope != r.scope {
		return nil, af.ErrDenied
	}
	return (servicebooking.Service{Pool: r.service.Pool}).Search(ctx, r.actor, query, limit)
}

func (r staffWorkflowReader) CheckAvailability(ctx context.Context, scope af.Scope, selection servicebooking.Selection) (servicebooking.Availability, error) {
	if scope != r.scope {
		return servicebooking.Availability{}, af.ErrDenied
	}
	return (servicebooking.Service{Pool: r.service.Pool}).Availability(ctx, r.actor, selection)
}

func (r staffWorkflowReader) SearchProperties(ctx context.Context, scope af.Scope, filters orders.PropertyFilter) ([]WorkflowProperty, error) {
	if scope != r.scope {
		return nil, af.ErrDenied
	}
	raw, err := (orders.Service{Pool: r.service.Pool}).SearchProperties(ctx, r.actor, filters)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []WorkflowProperty `json:"items"`
	}
	if len(raw) > af.MaxJSONBytes || json.Unmarshal(raw, &list) != nil || list.Items == nil {
		return nil, af.ErrInvalid
	}
	return list.Items, nil
}

func (b workflowBinding) execute(ctx context.Context, scope af.Scope, in WorkflowInput) (WorkflowResult, error) {
	if scope != b.scope || in.Scenario != b.scenario {
		return WorkflowResult{}, af.ErrDenied
	}
	out := WorkflowResult{Scenario: in.Scenario, State: "selection_required"}
	var err error
	switch in.Scenario {
	case WorkflowAutoParts:
		err = b.autoParts(ctx, &out, *in.AutoParts)
	case WorkflowConstruction:
		err = b.construction(ctx, &out, *in.Construction)
	case WorkflowServiceBooking:
		err = b.serviceBooking(ctx, &out, *in.ServiceBooking)
	case WorkflowPropertyViewing:
		err = b.propertyViewing(ctx, &out, *in.PropertyViewing)
	default:
		err = af.ErrInvalid
	}
	if err != nil {
		return WorkflowResult{}, err
	}
	if out.Draft != nil {
		out.State = "draft"
	}
	return out, nil
}

func (b workflowBinding) catalogSearch(ctx context.Context, out *WorkflowResult, search catalog.IndustrySearchFilters, selected string) (*WorkflowCatalogItem, error) {
	if search.Limit == 0 {
		search.Limit = workflowLimit
	}
	found, err := b.reader.SearchIndustry(ctx, b.scope, search)
	if err != nil {
		return nil, err
	}
	if len(found.Items) > search.Limit || !optionalWorkflowID(found.NextAfterVariantID) {
		return nil, af.ErrInvalid
	}
	result := &WorkflowCatalog{Items: make([]WorkflowCatalogItem, 0, len(found.Items)), NextAfterVariantID: found.NextAfterVariantID}
	seen := make(map[string]bool)
	var item *WorkflowCatalogItem
	for _, record := range found.Items {
		if !organizations.ValidID(record.ProductID) || !organizations.ValidID(record.VariantID) || seen[record.VariantID] {
			return nil, af.ErrInvalid
		}
		seen[record.VariantID] = true
		excerpt := WorkflowCatalogItem{ProductID: record.ProductID, VariantID: record.VariantID, Name: record.Name, SKU: record.SKU, Freshness: record.Freshness}
		if record.Freshness == "FRESH" && record.Available != nil && *record.Available >= 0 {
			units := *record.Available
			excerpt.Available = &units
		}
		if record.CurrencyCode == "RUB" && record.PriceAmount >= 0 {
			excerpt.Price = &WorkflowPrice{Amount: record.PriceAmount, Currency: record.CurrencyCode}
		}
		result.Items = append(result.Items, excerpt)
		if record.VariantID == selected {
			item = &excerpt
		}
	}
	out.Catalog = result
	if selected != "" && item == nil {
		out.State = "selection_not_in_results"
	}
	return item, nil
}

func (b workflowBinding) autoParts(ctx context.Context, out *WorkflowResult, in WorkflowAutoPartsInput) error {
	for _, field := range []struct{ key, value string }{
		{"make", in.Vehicle.Make}, {"model", in.Vehicle.Model}, {"engine", in.Vehicle.Engine},
	} {
		if strings.TrimSpace(field.value) == "" {
			out.Required = append(out.Required, field.key)
		}
	}
	if in.Vehicle.Year == 0 {
		out.Required = append(out.Required, "year")
	}
	if len(out.Required) > 0 {
		out.State = "clarification_required"
		return nil
	}
	search := in.Search
	search.Industry = "auto_parts"
	if in.VariantID == "" {
		search.Vehicle = &in.Vehicle
	}
	item, err := b.catalogSearch(ctx, out, search, in.VariantID)
	if err != nil || item == nil {
		return err
	}
	query := catalog.FitmentQuery{VariantID: item.VariantID, Make: in.Vehicle.Make, Model: in.Vehicle.Model, Year: in.Vehicle.Year, Engine: in.Vehicle.Engine}
	fitment, err := b.reader.CheckFitment(ctx, b.scope, query)
	if err != nil {
		return err
	}
	if fitment.VariantID != item.VariantID {
		return af.ErrDenied
	}
	status := fitment.Status
	switch status {
	case catalog.FitmentCompatible, catalog.FitmentIncompatible:
		if !currentWorkflowFitment(fitment, query.Year, time.Now()) {
			status = catalog.FitmentUnknown
		}
	case catalog.FitmentUnknown:
	default:
		return af.ErrInvalid
	}
	out.Fitment = &WorkflowFitment{VariantID: item.VariantID, Status: status}
	if status != catalog.FitmentCompatible {
		out.State = "fitment_" + string(status)
		return nil
	}
	if item.Available == nil || *item.Available < int64(in.Quantity) {
		out.State = "stock_unverified_or_insufficient"
		return nil
	}
	out.Draft = &WorkflowDraft{Kind: "product_quote", EstimatedTotal: estimateWorkflowPrice(item.Price, int64(in.Quantity)), Items: []orders.CartItem{{VariantID: item.VariantID, Quantity: in.Quantity}}}
	return nil
}

func currentWorkflowFitment(result catalog.FitmentResult, year int, now time.Time) bool {
	valid := false
	for _, evidence := range result.Evidence {
		if evidence.SourceRef == "" || evidence.YearFrom > year || evidence.YearTo < year ||
			evidence.VerifiedAt.IsZero() || evidence.VerifiedAt.After(now) || !evidence.ValidUntil.After(now) {
			continue
		}
		if evidence.Compatible != (result.Status == catalog.FitmentCompatible) {
			return false
		}
		valid = true
	}
	return valid
}

func (b workflowBinding) construction(ctx context.Context, out *WorkflowResult, in WorkflowConstructionInput) error {
	calculation, err := bf.CalculateConstruction(in.Calculation)
	if err != nil {
		return af.ErrInvalid
	}
	out.Calculation = &calculation
	search := in.Search
	search.Industry = "building_materials"
	perPack := in.Calculation.UnitsPerPackage
	if perPack > 0 {
		count := strconv.FormatInt(perPack, 10)
		search.Text = append(append([]catalog.IndustryTextFilter(nil), search.Text...), catalog.IndustryTextFilter{Field: "selling_unit", Match: catalog.IndustryExact, Value: "package"})
		search.Ranges = append(append([]catalog.IndustryRangeFilter(nil), search.Ranges...), catalog.IndustryRangeFilter{Field: "package_quantity", Min: count, Max: count})
	}
	item, err := b.catalogSearch(ctx, out, search, in.VariantID)
	if err != nil || item == nil {
		return err
	}
	if perPack == 0 || calculation.PackageCount <= 0 {
		out.State = "packaging_unverified"
		return nil
	}
	if item.Available == nil || *item.Available < calculation.PackageCount {
		out.State = "stock_unverified_or_insufficient"
		return nil
	}
	out.Draft = &WorkflowDraft{
		Kind: "quotation_request", EstimatedTotal: estimateWorkflowPrice(item.Price, calculation.PackageCount), Items: []orders.CartItem{{VariantID: item.VariantID, Quantity: int(calculation.PackageCount)}},
		Details: orders.IntakeDetails{Summary: fmt.Sprintf("Variant %s, estimated %d packs of %d pieces from supplied dimensions. Check material and layout before quoting.",
			item.VariantID, calculation.PackageCount, perPack)},
	}
	return nil
}

func (b workflowBinding) serviceBooking(ctx context.Context, out *WorkflowResult, in WorkflowServiceInput) error {
	offerings, err := b.reader.SearchOfferings(ctx, b.scope, in.Query, workflowLimit)
	if err != nil {
		return err
	}
	if len(offerings) > workflowLimit {
		return af.ErrLimit
	}
	var revision int64
	seen := make(map[string]bool)
	for _, offering := range offerings {
		if !organizations.ValidID(offering.ID) || seen[offering.ID] || offering.Revision < 1 || offering.Status != "active" {
			return af.ErrInvalid
		}
		seen[offering.ID] = true
		out.Offerings = append(out.Offerings, WorkflowOffering{ID: offering.ID, Name: offering.Name, Revision: offering.Revision, MinMinutes: offering.MinMinutes, MaxMinutes: offering.MaxMinutes})
		if in.Selection != nil && in.Selection.OfferingID == offering.ID {
			revision = offering.Revision
		}
	}
	if in.Selection == nil {
		return nil
	}
	if revision == 0 {
		out.State = "selection_not_in_results"
		return nil
	}
	if revision != in.Selection.OfferingRevision {
		out.State = "source_changed"
		return nil
	}
	if err := b.checkSlot(ctx, out, *in.Selection); err != nil || out.Availability == nil {
		return err
	}
	details := orders.IntakeDetails{Selection: &out.Availability.Selection}
	if in.Parts == nil {
		out.Draft = &WorkflowDraft{Kind: "service_booking", Details: details}
		return nil
	}
	out.State = "part_selection_required"
	if err := b.autoParts(ctx, out, *in.Parts); err != nil || out.Draft == nil {
		return err
	}
	details.Items = out.Draft.Items
	out.Draft = &WorkflowDraft{Kind: "hybrid_order", Items: details.Items, Details: details}
	return nil
}

func (b workflowBinding) checkSlot(ctx context.Context, out *WorkflowResult, selection servicebooking.Selection) error {
	slot, err := b.reader.CheckAvailability(ctx, b.scope, selection)
	if err != nil {
		switch {
		case workflowDomainError(err, 409, "SERVICE_UNAVAILABLE"):
			out.State = "slot_unavailable"
		case workflowDomainError(err, 409, "SOURCE_CHANGED"):
			out.State = "source_changed"
		default:
			return err
		}
		return nil
	}
	if !matchingWorkflowSelection(slot.Selection, selection) || slot.Available != (slot.State == "available_nonbinding") {
		return af.ErrDenied
	}
	if !slot.Available {
		out.State = "slot_unavailable"
		return nil
	}
	out.Availability = &slot
	return nil
}

func matchingWorkflowSelection(a, b servicebooking.Selection) bool {
	return a.OfferingID == b.OfferingID && a.OfferingRevision == b.OfferingRevision && a.Timezone == b.Timezone &&
		a.Units == b.Units && a.StartsAt.Equal(b.StartsAt) && a.EndsAt.Equal(b.EndsAt)
}

func (b workflowBinding) propertyViewing(ctx context.Context, out *WorkflowResult, in WorkflowPropertyInput) error {
	properties, err := b.reader.SearchProperties(ctx, b.scope, in.Search)
	if err != nil {
		return err
	}
	if len(properties) > 20 {
		return af.ErrLimit
	}
	if in.ListingID != "" {
		out.State = "selection_not_in_results"
	}
	now := time.Now()
	seen := make(map[string]bool)
	for _, item := range properties {
		if !organizations.ValidID(item.ID) || seen[item.ID] || item.AreaM2 == "" || item.VerifiedUntil.IsZero() || !optionalWorkflowID(item.ViewingOfferingID) {
			return af.ErrInvalid
		}
		seen[item.ID] = true
		switch item.Status {
		case "available":
			if !item.VerifiedUntil.After(now) {
				item.Status = "unknown"
			}
		case "unknown", "unavailable":
		default:
			return af.ErrInvalid
		}
		out.Properties = append(out.Properties, item)
		if item.ID != in.ListingID {
			continue
		}
		out.State = "listing_unverified_or_unavailable"
		if item.Status != "available" {
			continue
		}
		details := orders.IntakeDetails{ListingID: item.ID}
		if item.ViewingOfferingID != "" {
			out.State = "viewing_slot_required"
			if in.Selection == nil || in.Selection.OfferingID != item.ViewingOfferingID {
				continue
			}
			if err := b.checkSlot(ctx, out, *in.Selection); err != nil {
				return err
			}
			if out.Availability == nil {
				continue
			}
			details.Selection = &out.Availability.Selection
		} else if in.Selection != nil {
			out.State = "viewing_slot_unavailable"
			continue
		}
		out.Draft = &WorkflowDraft{Kind: "property_viewing_request", Details: details}
	}
	return nil
}

func estimateWorkflowPrice(price *WorkflowPrice, quantity int64) *WorkflowPrice {
	if price == nil || quantity <= 0 || price.Amount > math.MaxInt64/quantity {
		return nil
	}
	return &WorkflowPrice{Amount: price.Amount * quantity, Currency: price.Currency}
}
