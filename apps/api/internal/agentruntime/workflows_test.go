package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
)

const (
	workflowTestTenant       = "10000000-0000-4000-8000-000000000001"
	workflowTestPrincipal    = "10000000-0000-4000-8000-000000000002"
	workflowTestConversation = "10000000-0000-4000-8000-000000000003"
	workflowTestTask         = "10000000-0000-4000-8000-000000000004"
	workflowTestResource     = "10000000-0000-4000-8000-000000000005"
	workflowTestProduct      = "10000000-0000-4000-8000-000000000006"
)

func workflowFixture() af.Task {
	return af.Task{ID: workflowTestTask, Scope: af.Scope{
		TenantID: workflowTestTenant, PrincipalID: workflowTestPrincipal, ConversationID: workflowTestConversation,
	}}
}

type workflowTestStore struct {
	claimed  bool
	finish   af.Status
	events   []af.AuditEvent
	auditErr error
}

func (s *workflowTestStore) Claim(_ context.Context, task af.Task) (bool, error) {
	expected := workflowFixture()
	expected.AgentID = "customer"
	if task != expected {
		return false, af.ErrDenied
	}
	fresh := !s.claimed
	s.claimed = true
	return fresh, nil
}

func (s *workflowTestStore) Finish(_ context.Context, _ af.Task, status af.Status) error {
	s.finish = status
	return nil
}

func (s *workflowTestStore) Record(_ context.Context, event af.AuditEvent) error {
	s.events = append(s.events, event)
	return s.auditErr
}

type workflowTestReader struct {
	scope       af.Scope
	scenario    WorkflowScenario
	catalog     catalog.IndustrySearchResult
	fitment     catalog.FitmentResult
	offerings   []servicebooking.Offering
	slot        servicebooking.Availability
	slotErr     error
	properties  []WorkflowProperty
	lastFilters catalog.IndustrySearchFilters
	domainCalls int
}

func (r *workflowTestReader) CheckWorkflowScope(_ context.Context, scope af.Scope, scenario WorkflowScenario) error {
	if scope != r.scope || scenario != r.scenario {
		return af.ErrDenied
	}
	return nil
}

func (r *workflowTestReader) read(scope af.Scope) error {
	r.domainCalls++
	if scope != r.scope {
		return af.ErrDenied
	}
	return nil
}

func (r *workflowTestReader) SearchIndustry(_ context.Context, scope af.Scope, filters catalog.IndustrySearchFilters) (catalog.IndustrySearchResult, error) {
	r.lastFilters = filters
	return r.catalog, r.read(scope)
}

func (r *workflowTestReader) CheckFitment(_ context.Context, scope af.Scope, _ catalog.FitmentQuery) (catalog.FitmentResult, error) {
	return r.fitment, r.read(scope)
}

func (r *workflowTestReader) SearchOfferings(_ context.Context, scope af.Scope, _ string, _ int) ([]servicebooking.Offering, error) {
	return r.offerings, r.read(scope)
}

func (r *workflowTestReader) CheckAvailability(_ context.Context, scope af.Scope, _ servicebooking.Selection) (servicebooking.Availability, error) {
	return r.slot, errors.Join(r.read(scope), r.slotErr)
}

func (r *workflowTestReader) SearchProperties(_ context.Context, scope af.Scope, _ orders.PropertyFilter) ([]WorkflowProperty, error) {
	return r.properties, r.read(scope)
}

func workflowRun(t *testing.T, in WorkflowInput, reader *workflowTestReader) WorkflowResult {
	t.Helper()
	task := workflowFixture()
	reader.scope, reader.scenario = task.Scope, in.Scenario
	store := &workflowTestStore{}
	out, err := RunWorkflow(t.Context(), task, in, WorkflowDependencies{Reader: reader, Stores: WorkflowStores{Tasks: store, Audit: store}})
	if err != nil {
		t.Fatal(err)
	}
	if !store.claimed || store.finish != af.StatusSucceeded || len(store.events) != 3 ||
		store.events[0].ParamsHash == "" || store.events[0].ApprovalStatus != af.ApprovalNone {
		t.Fatalf("missing orchestration claim, completion or audit: %+v", store)
	}
	return out
}

func workflowCatalogFixture(freshness string) catalog.IndustrySearchResult {
	units := int64(5)
	return catalog.IndustrySearchResult{Items: []catalog.IndustrySearchItem{{
		ProductID: workflowTestProduct, VariantID: workflowTestResource, Name: "Synthetic variant", SKU: "test",
		Freshness: freshness, Available: &units,
	}}}
}

func TestWorkflowAutoParts(t *testing.T) {
	now := time.Now()
	in := WorkflowInput{Scenario: WorkflowAutoParts, AutoParts: &WorkflowAutoPartsInput{
		Vehicle:   catalog.FitmentVehicle{Make: "Auto", Model: "Test", Engine: "Test", Year: 2020},
		VariantID: workflowTestResource, Quantity: 2,
	}}
	reader := &workflowTestReader{catalog: workflowCatalogFixture("FRESH"), fitment: catalog.FitmentResult{
		VariantID: workflowTestResource, Status: catalog.FitmentCompatible,
		Evidence: []catalog.FitmentEvidence{{SourceRef: "synthetic", Compatible: true, YearFrom: 2020, YearTo: 2020,
			VerifiedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Hour)}},
	}}
	out := workflowRun(t, in, reader)
	if out.State != "draft" || out.Draft == nil || out.Draft.Kind != "product_quote" ||
		len(out.Draft.Items) != 1 || out.Draft.Items[0].Quantity != 2 || reader.domainCalls != 2 {
		t.Fatalf("unexpected auto parts result: %+v", out)
	}

	reader.catalog = workflowCatalogFixture("STALE")
	out = workflowRun(t, in, reader)
	if out.Draft != nil || out.State != "stock_unverified_or_insufficient" || out.Catalog.Items[0].Available != nil {
		t.Fatalf("stale stock produced an availability claim: %+v", out)
	}

	reader.fitment.Evidence[0].ValidUntil = now.Add(-time.Second)
	out = workflowRun(t, in, reader)
	if out.Draft != nil || out.State != "fitment_unknown" || out.Fitment.Status != catalog.FitmentUnknown {
		t.Fatalf("expired proof yielded compatibility: %+v", out)
	}
}

func TestWorkflowConstructionPackaging(t *testing.T) {
	in := WorkflowInput{Scenario: WorkflowConstruction, Construction: &WorkflowConstructionInput{
		VariantID: workflowTestResource,
		Calculation: bf.ConstructionInput{
			Operation: bf.ConstructionTiles, Length: bf.ConstructionMeasure{Value: "2", Unit: "m"},
			Width: bf.ConstructionMeasure{Value: "1", Unit: "m"}, ItemLength: bf.ConstructionMeasure{Value: "500", Unit: "mm"},
			ItemWidth: bf.ConstructionMeasure{Value: "50", Unit: "cm"}, UnitsPerPackage: 4, WastePercent: "10",
		},
	}}
	reader := &workflowTestReader{catalog: workflowCatalogFixture("FRESH")}
	out := workflowRun(t, in, reader)
	if out.Draft == nil || out.Draft.Kind != "quotation_request" || out.Draft.Items[0].Quantity != 3 ||
		out.Calculation.Quantity != "9" || len(out.Draft.Details.Items) != 0 || out.Draft.Details.Summary == "" {
		t.Fatalf("unexpected construction result: %+v", out)
	}
	f := reader.lastFilters
	if f.Industry != "building_materials" || len(f.Text) != 1 || len(f.Ranges) != 1 ||
		f.Text[0] != (catalog.IndustryTextFilter{Field: "selling_unit", Match: catalog.IndustryExact, Value: "package"}) ||
		f.Ranges[0] != (catalog.IndustryRangeFilter{Field: "package_quantity", Min: "4", Max: "4"}) {
		t.Fatalf("package was not checked at source: %+v", f)
	}
	in.Construction.Calculation.UnitsPerPackage = 0
	out = workflowRun(t, in, reader)
	if out.Draft != nil || out.State != "packaging_unverified" {
		t.Fatalf("missing packaging yielded a draft: %+v", out)
	}
}

func TestWorkflowServiceAvailability(t *testing.T) {
	start := time.Date(2031, 1, 1, 11, 0, 0, 0, time.UTC)
	sel := servicebooking.Selection{OfferingID: workflowTestResource, OfferingRevision: 3,
		StartsAt: start, EndsAt: start.Add(time.Hour), Timezone: "UTC", Units: 1}
	in := WorkflowInput{Scenario: WorkflowServiceBooking, ServiceBooking: &WorkflowServiceInput{Selection: &sel}}
	reader := &workflowTestReader{
		offerings: []servicebooking.Offering{{ID: workflowTestResource, Name: "Synthetic", Status: "active", Revision: 3}},
		slot:      servicebooking.Availability{Available: true, State: "available_nonbinding", Selection: sel},
	}
	out := workflowRun(t, in, reader)
	if out.Draft == nil || out.Draft.Kind != "service_booking" || out.Availability == nil ||
		!matchingWorkflowSelection(*out.Draft.Details.Selection, sel) {
		t.Fatalf("unexpected booking result: %+v", out)
	}
	reader.slotErr = auth.Failure(409, "SERVICE_UNAVAILABLE", "")
	out = workflowRun(t, in, reader)
	if out.Draft != nil || out.State != "slot_unavailable" {
		t.Fatalf("unavailable slot yielded a draft: %+v", out)
	}
}

func TestWorkflowPropertyFreshness(t *testing.T) {
	in := WorkflowInput{Scenario: WorkflowPropertyViewing, PropertyViewing: &WorkflowPropertyInput{ListingID: workflowTestResource}}
	reader := &workflowTestReader{properties: []WorkflowProperty{{ID: workflowTestResource, Title: "Synthetic", Locality: "Test",
		Rooms: 2, AreaM2: "32.10", Status: "available", VerifiedUntil: time.Now().Add(time.Hour)}}}
	out := workflowRun(t, in, reader)
	if out.Draft == nil || out.Draft.Kind != "property_viewing_request" || out.Draft.Details.ListingID != workflowTestResource ||
		out.Properties[0].AreaM2 != "32.10" {
		t.Fatalf("unexpected property result: %+v", out)
	}
	reader.properties[0].VerifiedUntil = time.Now().Add(-time.Second)
	out = workflowRun(t, in, reader)
	if out.Draft != nil || out.Properties[0].Status != "unknown" {
		t.Fatalf("expired listing yielded a draft: %+v", out)
	}
}

func TestWorkflowDeniesScopeAndAuditFailures(t *testing.T) {
	task := workflowFixture()
	in := WorkflowInput{Scenario: WorkflowPropertyViewing, PropertyViewing: &WorkflowPropertyInput{}}
	other := task.Scope
	other.TenantID = workflowTestResource
	reader := &workflowTestReader{scope: other, scenario: in.Scenario}
	store := &workflowTestStore{}
	deps := WorkflowDependencies{Reader: reader, Stores: WorkflowStores{Tasks: store, Audit: store}}
	if out, err := RunWorkflow(t.Context(), task, in, deps); !errors.Is(err, af.ErrDenied) ||
		out.State != "" || store.claimed || reader.domainCalls != 0 {
		t.Fatalf("cross-tenant attempt reached the plan: %+v %v", out, err)
	}
	reader.scope = task.Scope
	store.auditErr = errors.New("audit failure")
	if out, err := RunWorkflow(t.Context(), task, in, deps); !errors.Is(err, store.auditErr) ||
		out.State != "" || !store.claimed || reader.domainCalls != 0 {
		t.Fatalf("unrecorded attempt performed a read: %+v %v", out, err)
	}
	actor := catalog.Tenant{
		OrganizationID: task.Scope.TenantID, AgentTaskID: task.ID, AgentLease: workflowTestResource,
		Session: auth.Session{User: auth.User{ID: task.Scope.PrincipalID}, Hash: make([]byte, 32)},
	}
	if _, err := (Service{Pool: new(pgxpool.Pool)}).RunStaffWorkflow(t.Context(), actor, task, in, deps.Stores); !errors.Is(err, af.ErrDenied) {
		t.Fatalf("lease accepted as staff: %v", err)
	}
}

func TestValidateWorkflowInput(t *testing.T) {
	in := WorkflowInput{Scenario: WorkflowPropertyViewing, PropertyViewing: &WorkflowPropertyInput{}, ServiceBooking: &WorkflowServiceInput{}}
	if !errors.Is(ValidateWorkflowInput(in), af.ErrInvalid) {
		t.Fatal("accepted a sibling scenario payload")
	}
	in = WorkflowInput{Scenario: WorkflowAutoParts, AutoParts: &WorkflowAutoPartsInput{
		Search:  catalog.IndustrySearchFilters{Industry: "building_materials"},
		Vehicle: catalog.FitmentVehicle{Make: "Auto", Model: "Test", Engine: "Test", Year: 2020},
	}}
	if !errors.Is(ValidateWorkflowInput(in), af.ErrInvalid) {
		t.Fatal("accepted industry substitution")
	}
}
