//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
)

type instagramCatalogFixture struct {
	OrganizationID, ProductID, VariantID, SKU string
	Price                                     int64
}

func seedInstagramCatalog(t *testing.T, d *db.Database, ctx context.Context, tenant catalog.Tenant, index int) instagramCatalogFixture {
	t.Helper()
	svc := catalog.Service{Pool: d.Pool}
	f := instagramCatalogFixture{OrganizationID: tenant.OrganizationID, SKU: fmt.Sprintf("SYNTH-IG-%d", index), Price: int64(12345 + index*10000)}
	raw, err := svc.CreateProduct(ctx, tenant, catalog.Input{Name: "Synthetic Instagram catalog", SKU: f.SKU, PriceAmount: f.Price, CurrencyCode: "RUB"})
	if err != nil {
		t.Fatal("seed catalog", err)
	}
	var product struct {
		ID       string `json:"id"`
		Variants []struct {
			ID string `json:"id"`
		} `json:"variants"`
	}
	if json.Unmarshal(raw, &product) != nil || len(product.Variants) != 1 {
		t.Fatal("catalog fixture shape")
	}
	f.ProductID, f.VariantID = product.ID, product.Variants[0].ID
	if _, err = svc.Adjust(ctx, tenant, catalog.Input{VariantID: f.VariantID, QuantityOnHand: 7, ExpectedVersion: 1, Reason: "Synthetic routing fixture"}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.UpdateProduct(ctx, tenant, f.ProductID, catalog.Input{ExpectedVersion: 1}, "publish"); err != nil {
		t.Fatal(err)
	}
	return f
}

func assertInstagramCatalogResponse(t *testing.T, admin *pgx.Conn, ctx context.Context, org, conv string, own instagramCatalogFixture, all []instagramCatalogFixture) {
	t.Helper()
	var raw []byte
	if err := admin.QueryRow(ctx, `SELECT data FROM platform.messages WHERE organization_id=$1 AND conversation_id=$2 AND role='assistant' ORDER BY created_at DESC,id DESC LIMIT 1`, org, conv).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Cards []struct {
			VariantID string `json:"variant_id"`
			SKU       string `json:"sku"`
			Price     int64  `json:"price_amount"`
			Available *int   `json:"available"`
		} `json:"cards"`
	}
	if json.Unmarshal(raw, &response) != nil || len(response.Cards) != 1 {
		t.Fatalf("expected actual tenant catalog card, got %s", raw)
	}
	card := response.Cards[0]
	if card.VariantID != own.VariantID || card.SKU != own.SKU || card.Price != own.Price || card.Available == nil || *card.Available != 7 {
		t.Fatal("wrong catalog/price/stock routing")
	}
	for _, other := range all {
		if other.VariantID != own.VariantID && strings.Contains(string(raw), other.VariantID) {
			t.Fatal("foreign catalog in AI result")
		}
	}
}

func assertInstagramCatalogLease(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context, tenant catalog.Tenant, fixtures []instagramCatalogFixture) {
	t.Helper()
	const snapshot = `SELECT md5(coalesce((SELECT jsonb_agg(to_jsonb(p) ORDER BY id)::text FROM platform.products p),'')),md5(coalesce((SELECT jsonb_agg(to_jsonb(i) ORDER BY variant_id)::text FROM platform.inventory i),''))`
	var productsBefore, stockBefore, productsAfter, stockAfter string
	if err := admin.QueryRow(ctx, snapshot).Scan(&productsBefore, &stockBefore); err != nil {
		t.Fatal(err)
	}
	denied := func(err error, status int) {
		t.Helper()
		var failure *auth.Error
		if !errors.As(err, &failure) || failure.Status != status {
			t.Fatalf("expected authorization/not-found %d, got %v", status, err)
		}
	}
	reader := (catalog.Service{Pool: d.Pool}).Customer()
	for _, f := range fixtures {
		price, err := reader.GetProductPrice(ctx, tenant, f.VariantID)
		if f.VariantID == fixtures[0].VariantID {
			if err != nil {
				t.Fatal("Instagram leased catalog price", err)
			}
			var actual struct {
				Amount int64 `json:"price_amount"`
			}
			if json.Unmarshal(price, &actual) != nil || actual.Amount != f.Price {
				t.Fatal("wrong leased price")
			}
			if _, err = reader.CheckInventory(ctx, tenant, f.VariantID); err != nil {
				t.Fatal("Instagram leased stock", err)
			}
		} else {
			denied(err, 404)
		}
	}
	wrongChannel := tenant
	wrongChannel.ChannelType = "whatsapp"
	_, err := reader.GetProductPrice(ctx, wrongChannel, fixtures[0].VariantID)
	denied(err, 404)
	foreign := tenant
	foreign.OrganizationID = fixtures[1].OrganizationID
	_, err = reader.GetProductPrice(ctx, foreign, fixtures[1].VariantID)
	denied(err, 404)
	empty := tenant
	empty.ChannelCapability = ""
	_, err = reader.GetProductPrice(ctx, empty, fixtures[0].VariantID)
	denied(err, 404)
	_, err = (catalog.Service{Pool: d.Pool}).CreateProduct(ctx, tenant, catalog.Input{Name: "Forbidden channel write", SKU: "IG-DENIED-WRITE", PriceAmount: 100, CurrencyCode: "RUB"})
	denied(err, 403)
	if err = admin.QueryRow(ctx, snapshot).Scan(&productsAfter, &stockAfter); err != nil || productsBefore != productsAfter || stockBefore != stockAfter {
		t.Fatal("negative catalog access changed persistent data", err)
	}
}
