// Synthetic fixtures only. Never built into API/worker and never exposes HTTP.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"net/url"
	"os"
	"strings"
	"time"
)

type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}
type Shop struct {
	ID      string `json:"id"`
	Owner   User   `json:"owner"`
	Variant string `json:"variant"`
	Price   int    `json:"price"`
	Name    string `json:"name"`
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "Synthetic browser fixture failed")
		os.Exit(1)
	}
}
func run() error {
	u, e := url.Parse(os.Getenv("ABA_BROWSER_ADMIN_DATABASE_URL"))
	if e != nil || u.Hostname() != "postgres" || u.Path != "/aba_test" || u.User == nil || u.User.Username() != "postgres" {
		return fmt.Errorf("isolated fixture DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, e := pgx.Connect(ctx, u.String())
	if e != nil {
		return e
	}
	defer c.Close(ctx)
	if len(os.Args) == 2 && (os.Args[1] == "stage25" || os.Args[1] == "stage263") {
		return seedStage25(ctx, c, os.Args[1] == "stage263")
	}
	if len(os.Args) == 2 && os.Args[1] == "stage26" {
		return seedStage26(ctx, c)
	}
	if len(os.Args) == 3 && os.Args[1] == "demo" {
		return seedDemo(ctx, c, os.Args[2])
	}
	if len(os.Args) == 3 && os.Args[1] == "catalog-scale" {
		org := os.Args[2]
		tx, e := c.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		var owner, pid string
		if e = tx.QueryRow(ctx, `SELECT m.user_id,p.id FROM platform.organizations o JOIN platform.organization_members m ON m.organization_id=o.id AND m.role='owner' JOIN platform.products p ON p.organization_id=o.id WHERE o.id=$1 AND o.is_test AND o.name LIKE 'Synthetic8D%' ORDER BY p.id LIMIT 1`, org).Scan(&owner, &pid); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT set_config('app.catalog_writer','MANUAL',true)`); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,attributes,price_amount,currency_code) SELECT gen_random_uuid(),$1,$2,'SCALE-'||lpad(n::text,5,'0'),'Synthetic','{"synthetic":"true"}',10000,'RUB' FROM generate_series(1,6001) n ON CONFLICT DO NOTHING`, org, pid); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) SELECT $1,id,10,now() FROM platform.product_variants WHERE organization_id=$1 AND sku LIKE 'SCALE-%' ON CONFLICT DO NOTHING`, org); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.catalog_imports(id,organization_id,request_key,file_digest,grid,status,created_by,created_at,minimized_at) SELECT gen_random_uuid(),$1,gen_random_uuid(),sha256('synthetic browser scale'::bytea),'[]','FAILED',$2,'2020-01-01'::timestamptz,now() FROM generate_series(1,235)`, org, owner); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	if len(os.Args) == 4 && os.Args[1] == "expire-import" {
		org, job := os.Args[2], os.Args[3]
		tag, e := c.Exec(ctx, `UPDATE platform.catalog_imports SET created_at=now()-interval '10 days',expires_at=now()-interval '2 days' WHERE organization_id=$1 AND id=$2 AND status IN ('FAILED','CANCELLED') AND NOT retention_hold AND EXISTS(SELECT 1 FROM platform.organizations WHERE id=$1 AND is_test AND name LIKE 'Synthetic8D%')`, org, job)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("synthetic import required")
		}
		return nil
	}
	if len(os.Args) == 4 && os.Args[1] == "stock-state" {
		org, state := os.Args[2], os.Args[3]
		var valid bool
		if e = c.QueryRow(ctx, `SELECT is_test AND name LIKE 'Synthetic8D%' FROM platform.organizations WHERE id=$1`, org).Scan(&valid); e != nil || !valid {
			return fmt.Errorf("synthetic organization required")
		}
		var age string
		switch state {
		case "STALE":
			age = "100 hours"
		case "FRESH":
			age = "0 hours"
		case "UNKNOWN":
			age = ""
		case "DEMO":
			age = "100 hours"
		default:
			return fmt.Errorf("invalid synthetic state")
		}
		_, e = c.Exec(ctx, `UPDATE platform.inventory SET manual_updated_at=CASE WHEN $2='' OR ($3='DEMO' AND variant_id IN(SELECT id FROM platform.product_variants WHERE sku='DEMO-HEADPHONES')) THEN NULL WHEN $3='DEMO' AND variant_id NOT IN(SELECT id FROM platform.product_variants WHERE sku='DEMO-IPHONE') THEN manual_updated_at ELSE now()-$2::interval END WHERE organization_id=$1`, org, age, state)
		return e
	}
	if len(os.Args) > 1 && os.Args[1] == "wa-operator" {
		if len(os.Args) != 4 {
			return fmt.Errorf("arguments")
		}
		org, operator := os.Args[2], os.Args[3]
		tx, e := c.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		var integration, conv string
		if e = tx.QueryRow(ctx, `SELECT i.id,p.conversation_id FROM platform.channel_integrations i JOIN platform.organizations o ON o.id=i.organization_id JOIN platform.channel_principals p ON p.integration_id=i.id WHERE o.id=$1 AND o.is_test AND o.name LIKE 'Synthetic8D%' AND i.mode='mock' AND EXISTS(SELECT 1 FROM platform.users WHERE id=$2 AND display_name='operator' AND email LIKE 'operator-%@example.test') LIMIT 1`, org, operator).Scan(&integration, &conv); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.wa_operator_grants(organization_id,user_id,expires_at,assigned_by) VALUES($1,$2,now()+interval '1 hour','control:synthetic-browser') ON CONFLICT DO NOTHING`, org, operator); e != nil {
			return e
		}
		price, outbox, res := auth.UUID(), auth.UUID(), auth.UUID()
		if _, e = tx.Exec(ctx, `INSERT INTO platform.wa_pricing(id,organization_id,integration_id,policy_version,region,category,message_type,billing_basis,currency,max_charge,source,verified_at,verified_until,effective_at,verified,synthetic) VALUES($1,$2,$3,'SYNTHETIC_BROWSER_UNKNOWN','ZZ','SERVICE','text','verified_maximum','RUB',0,'SYNTHETIC_FIXTURE',now(),now()+interval '1 hour',now(),false,true)`, price, org, integration); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.channel_outbox(id,organization_id,integration_id,conversation_id,external_user_id,role,payload,status) SELECT $1,$2,$3,$4,external_user_id,'staff','{"text":"SYNTHETIC reconciliation fixture - never sent"}','delivery_unknown' FROM platform.channel_principals WHERE conversation_id=$4 AND integration_id=$3`, outbox, org, integration, conv); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.wa_reservations(id,organization_id,integration_id,outbox_id,attempt,capability,pricing_id,amount,status,synthetic) VALUES($1,$2,$3,$4,1,$5,$6,0,'unknown',true)`, res, org, integration, outbox, auth.UUID(), price); e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"reservation": res})
	}
	if len(os.Args) > 1 && os.Args[1] == "wa-safety" {
		if len(os.Args) != 3 {
			return fmt.Errorf("arguments")
		}
		org := os.Args[2]
		tx, e := c.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		var integration, waba string
		if e = tx.QueryRow(ctx, `SELECT i.id,i.business_id FROM platform.channel_integrations i JOIN platform.organizations o ON o.id=i.organization_id WHERE o.id=$1 AND o.is_test AND o.name LIKE 'Synthetic8D%' AND i.mode='mock'`, org).Scan(&integration, &waba); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.wa_budgets(organization_id) VALUES($1) ON CONFLICT DO NOTHING`, org); e != nil {
			return e
		}
		for _, state := range []string{"APPROVED", "PENDING", "PAUSED"} {
			if _, e = tx.Exec(ctx, `INSERT INTO platform.wa_templates(id,organization_id,integration_id,waba_id,name,language,category,status,components,parameter_schema,verified_at,verified_until,source,policy_version,synthetic) VALUES($1,$2,$3,$4,$5,'ru','UTILITY',$6,'[{"type":"BODY","text":"Synthetic {{1}}"}]','["text"]',now(),now()+interval '1 hour','SYNTHETIC_BROWSER_FIXTURE','synthetic-browser-v1',true) ON CONFLICT DO NOTHING`, auth.UUID(), org, integration, waba, "browser_synthetic_"+strings.ToLower(state), state); e != nil {
				return e
			}
		}
		return tx.Commit(ctx)
	}
	if len(os.Args) > 1 && os.Args[1] == "expire-window" {
		if len(os.Args) != 4 {
			return fmt.Errorf("arguments")
		}
		conv, actor := os.Args[2], os.Args[3]
		tx, e := c.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		var org, integration string
		e = tx.QueryRow(ctx, `SELECT o.id,p.integration_id FROM platform.channel_principals p JOIN platform.organizations o ON o.id=p.organization_id JOIN platform.organization_members m ON m.organization_id=o.id WHERE p.conversation_id=$1 AND o.is_test AND o.name LIKE 'Synthetic8D%' AND m.user_id=$2 AND (m.role='owner' OR m.can_manage_orders)`, conv, actor).Scan(&org, &integration)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,8210))`, integration); e != nil {
			return e
		}
		id := auth.UUID()
		if _, e = tx.Exec(ctx, `INSERT INTO platform.messages(id,organization_id,conversation_id,actor_id,role,content,data,client_key) VALUES($1,$2,$3,$4,'staff','Synthetic queue before expiry','{}',$1)`, id, org, conv, actor); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE platform.conversations SET message_count=message_count+1,version=version+1,updated_at=now() WHERE id=$1`, conv); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE platform.channel_principals SET window_until=now()-interval '1 hour' WHERE conversation_id=$1`, conv); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	tx, e := c.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	nonce := strings.ReplaceAll(auth.UUID(), "-", "")
	pw := "Synthetic-Phase2-Password-123!"
	hash := auth.PasswordHash(pw)
	user := func(label string) (User, error) {
		v := User{auth.UUID(), label + "-" + nonce + "@example.test", pw}
		_, err := tx.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,$3,$4,now())`, v.ID, v.Email, hash, label)
		return v, err
	}
	shops := []Shop{}
	for i := 0; i < 3; i++ {
		a, e := user(fmt.Sprintf("owner%d", i))
		if e != nil {
			return e
		}
		o := Shop{ID: auth.UUID(), Owner: a, Variant: auth.UUID(), Price: (i + 1) * 110099, Name: fmt.Sprintf("Synthetic8D Shop %c", 'A'+i)}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.organizations(id,name,is_test) VALUES($1,$2,true)`, o.ID, o.Name); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'owner',true)`, o.ID, a.ID); e != nil {
			return e
		}
		product := auth.UUID()
		if _, e = tx.Exec(ctx, `INSERT INTO platform.products(id,organization_id,name,brand,status) VALUES($1,$2,'Synthetic Galaxy P8D','Samsung','active')`, product, o.ID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,price_amount) VALUES($1,$2,$3,'P8D','Основной',$4)`, o.Variant, o.ID, product, o.Price); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) VALUES($1,$2,10,now())`, o.ID, o.Variant); e != nil {
			return e
		}
		shops = append(shops, o)
	}
	staff, e := user("staff")
	if e != nil {
		return e
	}
	member, e := user("readonly")
	if e != nil {
		return e
	}
	for _, v := range []struct {
		u     User
		write bool
	}{{staff, true}, {member, false}} {
		if _, e = tx.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',$3)`, shops[0].ID, v.u.ID, v.write); e != nil {
			return e
		}
	}
	operator, e := user("operator")
	if e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"shops": shops, "staff": staff, "member": member, "operator": operator})
}
