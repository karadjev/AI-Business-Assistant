package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
)

// seedStage25 runs only via the fixture's isolated postgres/aba_test DSN guard.
// It uses independent synthetic organizations and never alters Stage 2.4 seeds.
func seedStage25(ctx context.Context, c *pgx.Conn) error {
	var safe bool
	if err := c.QueryRow(ctx, `SELECT current_database()='aba_test' AND EXISTS(SELECT 1 FROM platform.test_safety WHERE marker='aba-synthetic-isolated') AND to_regclass('platform.service_offerings') IS NOT NULL`).Scan(&safe); err != nil || !safe {
		return fmt.Errorf("isolated stage25 database required")
	}
	tx, err := c.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	password := "Synthetic-Stage25-Password-123!"
	hashed := auth.PasswordHash(password)
	result := map[string]any{}
	type business struct {
		ID           string `json:"id"`
		Owner        User   `json:"owner"`
		Variant      string `json:"variant"`
		Conversation string `json:"conversation"`
		Offering     string `json:"offering"`
		Listing      string `json:"listing"`
		StartsAt     string `json:"starts_at"`
		EndsAt       string `json:"ends_at"`
	}
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
	end := start.Add(time.Hour)
	for _, kind := range []string{"auto_parts", "building_materials", "vehicle_service", "real_estate"} {
		model := "trade"
		if kind == "vehicle_service" {
			model = "service"
		}
		if kind == "real_estate" {
			model = "agency"
		}
		b := business{ID: auth.UUID(), Owner: User{ID: auth.UUID(), Email: "stage25-" + strings.ReplaceAll(auth.UUID(), "-", "") + "@example.test", Password: password}, Variant: auth.UUID(), Conversation: auth.UUID(), Offering: auth.UUID(), Listing: auth.UUID(), StartsAt: start.Format(time.RFC3339), EndsAt: end.Format(time.RFC3339)}
		if _, err = tx.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,$3,'Synthetic25 owner',now())`, b.Owner.ID, b.Owner.Email, hashed); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO platform.organizations(id,name,is_test,business_model,industry_profile) VALUES($1,$2,true,$3,$4)`, b.ID, "Synthetic25 "+kind, model, kind); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'owner',true)`, b.ID, b.Owner.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO platform.conversations(id,organization_id,customer_id,created_by) VALUES($1,$2,$3,$4)`, b.Conversation, b.ID, auth.UUID(), b.Owner.ID); err != nil {
			return err
		}
		if model == "trade" {
			product := auth.UUID()
			if _, err = tx.Exec(ctx, `INSERT INTO platform.products(id,organization_id,name,brand,status) VALUES($1,$2,$3,'Synthetic25','active')`, product, b.ID, "Synthetic25 "+kind); err != nil {
				return err
			}
			attrs := `{"oem":"SYN-25-OEM","manufacturer":"Synthetic25"}`
			if kind == "building_materials" {
				attrs = `{"material":"ceramic","selling_unit":"package","package_quantity":"8","weight_kg":"12.5"}`
			}
			if _, err = tx.Exec(ctx, `INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,attributes,price_amount) VALUES($1,$2,$3,'SYN25','Synthetic25',$4::jsonb,12500)`, b.Variant, b.ID, product, attrs); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) VALUES($1,$2,10,now())`, b.ID, b.Variant); err != nil {
				return err
			}
			if kind == "auto_parts" {
				if _, err = tx.Exec(ctx, `INSERT INTO platform.catalog_fitment_evidence(id,organization_id,variant_id,make,model,engine,year_from,year_to,compatible,source_ref,verified_at,valid_until) VALUES($1,$2,$3,'synthetic','model25','e25',2020,2030,true,'synthetic:stage25',now()-interval '1 hour',now()+interval '2 days')`, auth.UUID(), b.ID, b.Variant); err != nil {
					return err
				}
			}
		}
		if model == "service" {
			resource := auth.UUID()
			if _, err = tx.Exec(ctx, `INSERT INTO platform.service_offerings(id,organization_id,name,status,price_model,price_state,amount_minor,min_minutes,max_minutes,timezone) VALUES($1,$2,'Synthetic25 inspection','active','fixed','known',50000,60,60,'UTC')`, b.Offering, b.ID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO platform.service_resources(id,organization_id,name,kind,capacity) VALUES($1,$2,'Synthetic25 bay','equipment',1)`, resource, b.ID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO platform.service_offering_resources(organization_id,offering_id,resource_id) VALUES($1,$2,$3)`, b.ID, b.Offering, resource); err != nil {
				return err
			}
			for _, layer := range []string{"business", "offering", "shift"} {
				if _, err = tx.Exec(ctx, `INSERT INTO platform.service_availability_windows(organization_id,offering_id,resource_id,layer,starts_at,ends_at,timezone) VALUES($1,$2,$3,$4,$5,$6,'UTC')`, b.ID, b.Offering, resource, layer, start.Add(-time.Hour), end.Add(time.Hour)); err != nil {
					return err
				}
			}
		}
		if model == "agency" {
			if _, err = tx.Exec(ctx, `INSERT INTO platform.property_listings(id,organization_id,title,locality,rooms,area_m2,status,verified_until) VALUES($1,$2,'Synthetic25 current listing','Synthetic city',2,64.5,'available',now()+interval '2 days'),($3,$2,'Synthetic25 expired listing','Synthetic city',1,32,'available',now()-interval '1 hour')`, b.Listing, b.ID, auth.UUID()); err != nil {
				return err
			}
		}
		result[kind] = b
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
