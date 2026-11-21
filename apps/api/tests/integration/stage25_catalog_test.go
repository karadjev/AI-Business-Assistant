//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
)

func testStage25Catalog(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	t.Helper()
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	trade := seedStage23Owner(t, d, admin, ctx, identity, 250).tenant
	auto := seedStage23Owner(t, d, admin, ctx, identity, 251).tenant
	foreign := seedStage23Owner(t, d, admin, ctx, identity, 252).tenant
	svc := catalog.Service{Pool: d.Pool}
	customer := svc.Customer()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE platform.organizations SET industry_profile='building_materials' WHERE id=$1`, trade.OrganizationID)
	exec(`UPDATE platform.organizations SET industry_profile='auto_parts' WHERE id IN ($1,$2)`, auto.OrganizationID, foreign.OrganizationID)

	addProduct := func(tenant catalog.Tenant, name, sku string, attrs map[string]string, stock int64, publish bool) string {
		t.Helper()
		raw, err := svc.CreateProduct(ctx, tenant, catalog.Input{Name: name, SKU: sku, Attributes: attrs, PriceAmount: 7500})
		if err != nil {
			t.Fatal(err)
		}
		var product struct {
			ID       string
			Variants []struct{ ID string }
		}
		if err = json.Unmarshal(raw, &product); err != nil || len(product.Variants) != 1 {
			t.Fatalf("decode catalog fixture: %v", err)
		}
		variant := product.Variants[0].ID
		if _, err = svc.Adjust(ctx, tenant, catalog.Input{VariantID: variant, QuantityOnHand: stock, ExpectedVersion: 1, Reason: "Synthetic stage25 fixture"}); err != nil {
			t.Fatal(err)
		}
		if publish {
			if _, err = svc.UpdateProduct(ctx, tenant, product.ID, catalog.Input{ExpectedVersion: 1}, "publish"); err != nil {
				t.Fatal(err)
			}
		}
		return variant
	}
	fresh := addProduct(trade, "Stage25% brick", "STAGE25-FRESH", map[string]string{"weight_kg": "12.500"}, 8, true)
	stale := addProduct(trade, "Stage25 stale brick", "STAGE25-STALE", map[string]string{"weight_kg": "12.500"}, 9, true)
	empty := addProduct(trade, "Stage25 empty brick", "STAGE25-EMPTY", map[string]string{"weight_kg": "12.500000001"}, 0, true)
	addProduct(trade, "Stage25 invalid legacy weight", "STAGE25-LEGACY", map[string]string{"weight_kg": "12.500 kg"}, 8, true)
	addProduct(trade, "Stage25 draft brick", "STAGE25-DRAFT", map[string]string{"weight_kg": "12.500"}, 8, false)
	exec(`UPDATE platform.inventory SET manual_updated_at=now()-interval '100 hours' WHERE organization_id=$1 AND variant_id=$2`, trade.OrganizationID, stale)

	expectStatus := func(err error, status int) {
		t.Helper()
		var apiErr *auth.Error
		if !errors.As(err, &apiErr) || apiErr.Status != status {
			t.Fatalf("wanted HTTP %d, got %v", status, err)
		}
	}
	search := func(f catalog.IndustrySearchFilters) catalog.IndustrySearchResult {
		t.Helper()
		result, err := customer.SearchIndustry(ctx, trade, f)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	filters := catalog.IndustrySearchFilters{
		Industry: "building_materials",
		Text:     []catalog.IndustryTextFilter{{Field: "name", Match: catalog.IndustryPartial, Value: "stage25"}},
		Ranges:   []catalog.IndustryRangeFilter{{Field: "weight_kg", Min: "12.500", Max: "12.500"}},
	}
	filters.Availability = catalog.IndustryInStock
	items := search(filters).Items
	if len(items) != 1 || items[0].VariantID != fresh || items[0].Available == nil || *items[0].Available != 8 {
		t.Fatalf("bounded fresh search: %+v", items)
	}
	filters.Availability = catalog.IndustryUnknown
	items = search(filters).Items
	if len(items) != 1 || items[0].VariantID != stale || items[0].Available != nil || items[0].Freshness != "STALE" {
		t.Fatalf("stale stock projection: %+v", items)
	}
	filters.Availability = catalog.IndustryOutOfStock
	filters.Ranges = []catalog.IndustryRangeFilter{{Field: "weight_kg", Min: "12.500000001"}, {Field: "price_amount", Max: "7500"}}
	items = search(filters).Items
	if len(items) != 1 || items[0].VariantID != empty || items[0].Available == nil || *items[0].Available != 0 {
		t.Fatalf("fresh zero stock projection: %+v", items)
	}
	filters.Availability = ""
	filters.Ranges = nil
	filters.Text = []catalog.IndustryTextFilter{{Field: "name", Match: catalog.IndustryPartial, Value: "stage25%"}}
	items = search(filters).Items
	if len(items) != 1 || items[0].VariantID != fresh {
		t.Fatalf("literal prefix: %+v", items)
	}
	filters.Text[0].Value = "stage25"
	filters.Limit = 1
	page := search(filters)
	if len(page.Items) != 1 || page.NextAfterVariantID != page.Items[0].VariantID {
		t.Fatalf("first page: %+v", page)
	}
	filters.AfterVariantID = page.NextAfterVariantID
	next := search(filters)
	if len(next.Items) != 1 || next.Items[0].VariantID == page.Items[0].VariantID {
		t.Fatalf("cursor repeated or omitted a row: %+v", next)
	}
	filters.Ranges = []catalog.IndustryRangeFilter{{Field: "weight_kg", Min: "2.01", Max: "2.00"}}
	_, err := customer.SearchIndustry(ctx, trade, filters)
	expectStatus(err, http.StatusUnprocessableEntity)
	filters.Ranges = []catalog.IndustryRangeFilter{{Field: "memory_gb", Min: "16"}}
	_, err = customer.SearchIndustry(ctx, trade, filters)
	expectStatus(err, http.StatusUnprocessableEntity)
	filters.Ranges = nil
	filters.Industry = "auto_parts"
	_, err = customer.SearchIndustry(ctx, trade, filters)
	expectStatus(err, http.StatusNotFound)

	part := addProduct(auto, "Stage25 auto part", "STAGE25-PART", map[string]string{"oem": "SYNTH-25"}, 1, true)
	another := addProduct(auto, "Stage25 compatible spare", "STAGE25-SPARE", nil, 3, true)
	unpublished := addProduct(auto, "Stage25 unpublished part", "STAGE25-UNPUBLISHED", nil, 1, false)
	oem, err := svc.SearchIndustry(ctx, auto, catalog.IndustrySearchFilters{Industry: "auto_parts", Text: []catalog.IndustryTextFilter{
		{Field: "oem", Match: catalog.IndustryExact, Value: "synth-25"},
	}})
	if err != nil || len(oem.Items) != 1 || oem.Items[0].VariantID != part {
		t.Fatalf("canonical OEM search: %+v, %v", oem, err)
	}

	query := catalog.FitmentQuery{VariantID: part, Make: "SYNTHETIC", Model: "Series A", Year: 2023, Engine: "S25"}
	check := func(q catalog.FitmentQuery, status catalog.FitmentStatus, evidence int) catalog.FitmentResult {
		t.Helper()
		result, err := customer.CheckFitment(ctx, auto, q)
		if err != nil || result.Status != status || len(result.Evidence) != evidence {
			t.Fatalf("fitment wanted %s with %d records, got %+v: %v", status, evidence, result, err)
		}
		return result
	}
	// An exact OEM hit alone must never imply compatibility.
	check(query, catalog.FitmentUnknown, 0)
	seedEvidence := func(variant, engine, source string, compatible bool, lifecycle string) {
		t.Helper()
		exec(`INSERT INTO platform.catalog_fitment_evidence
 (id,organization_id,variant_id,make,model,engine,year_from,year_to,compatible,source_ref,verified_at,valid_until,revoked_at)
 VALUES($1,$2,$3,'synthetic','series a',$4,2021,2024,$5,$6,now()-interval '2 days',
 CASE WHEN $7='expired' THEN now()-interval '1 day' ELSE now()+interval '1 day' END,
 CASE WHEN $7='revoked' THEN now()-interval '1 day' END)`, auth.UUID(), auto.OrganizationID, variant, engine, compatible, source, lifecycle)
	}
	seedEvidence(part, "s25", "synthetic:positive", true, "valid")
	seedEvidence(part, "s25", "synthetic:revoked", false, "revoked")
	seedEvidence(part, "s25", "synthetic:expired", false, "expired")
	if result := check(query, catalog.FitmentCompatible, 1); result.Evidence[0].SourceRef != "synthetic:positive" {
		t.Fatalf("wrong applicability source: %+v", result)
	}
	vehicle := catalog.FitmentVehicle{Make: query.Make, Model: query.Model, Year: query.Year, Engine: query.Engine}
	vehicleFilters := catalog.IndustrySearchFilters{Industry: "auto_parts", Vehicle: &vehicle, Limit: 1, Availability: catalog.IndustryInStock}
	searchVehicle := func() catalog.IndustrySearchResult {
		t.Helper()
		found, err := customer.SearchIndustry(ctx, auto, vehicleFilters)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	seedEvidence(another, "s25", "synthetic:alternate", true, "valid")
	vehiclePage := searchVehicle()
	if len(vehiclePage.Items) != 1 || vehiclePage.NextAfterVariantID != vehiclePage.Items[0].VariantID {
		t.Fatalf("vehicle search page: %+v", vehiclePage)
	}
	vehicleFilters.AfterVariantID = vehiclePage.NextAfterVariantID
	vehicleNext := searchVehicle()
	if len(vehicleNext.Items) != 1 || vehicleNext.Items[0].VariantID == vehiclePage.Items[0].VariantID ||
		vehicleNext.Items[0].PriceAmount != 7500 || vehicleNext.Items[0].Available == nil || vehicleNext.NextAfterVariantID != "" {
		t.Fatalf("vehicle search continuation: %+v", vehicleNext)
	}
	vehicleFilters.AfterVariantID = ""
	changed := query
	changed.Year = 2025
	check(changed, catalog.FitmentUnknown, 0)
	changed = query
	changed.Engine = "S26"
	check(changed, catalog.FitmentUnknown, 0)
	seedEvidence(part, "s26", "synthetic:negative", false, "valid")
	check(changed, catalog.FitmentIncompatible, 1)
	seedEvidence(part, "s25", "synthetic:conflict", false, "valid")
	if result := check(query, catalog.FitmentUnknown, 2); result.Reason != "conflicting_verified_evidence" {
		t.Fatalf("conflicting applicability: %+v", result)
	}
	filtered := searchVehicle()
	if len(filtered.Items) != 1 || filtered.Items[0].VariantID != another || filtered.NextAfterVariantID != "" {
		t.Fatalf("vehicle search returned conflicting evidence: %+v", filtered)
	}
	_, err = customer.CheckFitment(ctx, foreign, query)
	expectStatus(err, http.StatusNotFound)
	changed = query
	changed.VariantID = unpublished
	_, err = customer.CheckFitment(ctx, auto, changed)
	expectStatus(err, http.StatusNotFound)
}
