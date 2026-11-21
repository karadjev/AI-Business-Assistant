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

type stage26Slot struct {
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
}
type stage26Fixture struct {
	ID                  string                 `json:"id"`
	Owner               User                   `json:"owner"`
	Staff               User                   `json:"staff"`
	Member              User                   `json:"member"`
	Outsider            User                   `json:"outsider"`
	Conversation        string                 `json:"conversation"`
	Offering            string                 `json:"offering"`
	Resource            string                 `json:"resource"`
	Variants            map[string]string      `json:"variants"`
	Slots               map[string]stage26Slot `json:"slots"`
	ForeignVariant      string                 `json:"foreign_variant"`
	ForeignConversation string                 `json:"foreign_conversation"`
}

// Synthetic-only seed: the caller already enforces postgres/aba_test DSN, and
// this command independently requires the isolated database marker. All existing
// Stage 2.4/2.5 organizations, products, users and schedules remain unchanged.
func seedStage26(ctx context.Context, c *pgx.Conn) error {
	var safe bool
	if err := c.QueryRow(ctx, `SELECT current_database()='aba_test' AND EXISTS(SELECT 1 FROM platform.test_safety WHERE marker='aba-synthetic-isolated') AND to_regclass('platform.service_offerings') IS NOT NULL`).Scan(&safe); err != nil || !safe {
		return fmt.Errorf("isolated stage26 database required")
	}
	tx, err := c.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	password := "Synthetic-Stage26-Password-123!"
	hashed := auth.PasswordHash(password)
	user := func(label string) (User, error) {
		u := User{ID: auth.UUID(), Email: "stage26-" + label + "-" + strings.ReplaceAll(auth.UUID(), "-", "") + "@example.test", Password: password}
		_, err := tx.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,$3,$4,now())`, u.ID, u.Email, hashed, "Synthetic26 "+label)
		return u, err
	}
	f := stage26Fixture{ID: auth.UUID(), Conversation: auth.UUID(), Offering: auth.UUID(), Variants: map[string]string{}, Slots: map[string]stage26Slot{}, ForeignVariant: auth.UUID(), ForeignConversation: auth.UUID()}
	if f.Owner, err = user("owner"); err != nil {
		return err
	}
	if f.Staff, err = user("staff"); err != nil {
		return err
	}
	if f.Member, err = user("member"); err != nil {
		return err
	}
	if f.Outsider, err = user("outsider"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.organizations(id,name,is_test,business_model,industry_profile) VALUES($1,'Synthetic26 hybrid',true,'hybrid','mixed_nonfood')`, f.ID); err != nil {
		return err
	}
	for _, m := range []struct {
		u      User
		role   string
		manage bool
	}{{f.Owner, "owner", true}, {f.Staff, "member", true}, {f.Member, "member", false}} {
		if _, err = tx.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,$3,$4)`, f.ID, m.u.ID, m.role, m.manage); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.conversations(id,organization_id,customer_id,created_by) VALUES($1,$2,$3,$4)`, f.Conversation, f.ID, auth.UUID(), f.Owner.ID); err != nil {
		return err
	}
	for _, label := range []string{"slot_a", "slot_b", "stock", "staff"} {
		product, variant := auth.UUID(), auth.UUID()
		f.Variants[label] = variant
		stock := 2
		if label == "stock" {
			stock = 1
		}
		if _, err = tx.Exec(ctx, `INSERT INTO platform.products(id,organization_id,name,brand,status) VALUES($1,$2,$3,'Synthetic26','active')`, product, f.ID, "Synthetic26 "+label); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,price_amount) VALUES($1,$2,$3,$4,'Synthetic26',12500)`, variant, f.ID, product, "SYN26-"+strings.ToUpper(label)); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) VALUES($1,$2,$3,now())`, f.ID, variant, stock); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE platform.product_variants SET attributes='{"oem":"SYN-262-PART"}' WHERE organization_id=$1 AND id=$2`, f.ID, f.Variants["staff"]); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.catalog_fitment_evidence(id,organization_id,variant_id,make,model,engine,year_from,year_to,compatible,source_ref,verified_at,valid_until)
	VALUES(gen_random_uuid(),$1,$2,'synthetic','model25','e25',2020,2030,true,'synthetic:stage262',now()-interval '1 hour',now()+interval '2 days')`, f.ID, f.Variants["staff"]); err != nil {
		return err
	}
	resource := auth.UUID()
	f.Resource = resource
	if _, err = tx.Exec(ctx, `INSERT INTO platform.service_offerings(id,organization_id,name,status,price_model,price_state,amount_minor,min_minutes,max_minutes,timezone) VALUES($1,$2,'Synthetic26 installation','active','fixed','known',50000,60,60,'UTC')`, f.Offering, f.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.service_resources(id,organization_id,name,kind,capacity) VALUES($1,$2,'Synthetic26 installation bay','equipment',1)`, resource, f.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.service_offering_resources(organization_id,offering_id,resource_id) VALUES($1,$2,$3)`, f.ID, f.Offering, resource); err != nil {
		return err
	}
	start := time.Now().UTC().Add(72 * time.Hour).Truncate(24 * time.Hour).Add(9 * time.Hour)
	for index, label := range []string{"slot", "stock_a", "stock_b", "staff"} {
		at := start.Add(time.Duration(index) * 3 * time.Hour)
		f.Slots[label] = stage26Slot{at.Format(time.RFC3339), at.Add(time.Hour).Format(time.RFC3339)}
	}
	for _, layer := range []string{"business", "offering", "shift"} {
		if _, err = tx.Exec(ctx, `INSERT INTO platform.service_availability_windows(organization_id,offering_id,resource_id,layer,starts_at,ends_at,timezone) VALUES($1,$2,$3,$4,$5,$6,'UTC')`, f.ID, f.Offering, resource, layer, start.Add(-time.Hour), start.Add(12*time.Hour)); err != nil {
			return err
		}
	}
	foreignOrg, foreignProduct := auth.UUID(), auth.UUID()
	if _, err = tx.Exec(ctx, `INSERT INTO platform.organizations(id,name,is_test,business_model,industry_profile) VALUES($1,'Synthetic26 foreign',true,'hybrid','mixed_nonfood')`, foreignOrg); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'owner',true)`, foreignOrg, f.Outsider.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.conversations(id,organization_id,customer_id,created_by) VALUES($1,$2,$3,$4)`, f.ForeignConversation, foreignOrg, auth.UUID(), f.Outsider.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.products(id,organization_id,name,brand,status) VALUES($1,$2,'Synthetic26 foreign product','Synthetic26','active')`, foreignProduct, foreignOrg); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,price_amount) VALUES($1,$2,$3,'SYN26-FOREIGN','Synthetic26',12500)`, f.ForeignVariant, foreignOrg, foreignProduct); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) VALUES($1,$2,2,now())`, foreignOrg, f.ForeignVariant); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(f)
}
