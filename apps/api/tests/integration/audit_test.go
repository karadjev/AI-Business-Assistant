//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

func testAudit8A(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	owner := func() catalog.Tenant {
		uid, token := auth.UUID(), auth.Token()
		if _, e := admin.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Audit',now())`, uid, uid+"@audit.example.test"); e != nil {
			t.Fatal(e)
		}
		if _, e := admin.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(auth.Token())); e != nil {
			t.Fatal(e)
		}
		s, e := identity.Authenticate(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
		o, e := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, s, "Synthetic audit")
		if e != nil {
			t.Fatal(e)
		}
		return catalog.Tenant{Session: s, OrganizationID: o.ID}
	}
	a, b := owner(), owner()
	chat := conversations.Service{Pool: d.Pool, Config: cfg, Provider: agent.Mock{}}
	tc := config.Telegram{Mode: "mock", Key: bytes.Repeat([]byte{31}, 32), Enabled: true, Retention: 7 * 24 * time.Hour}
	tg := telegram.Service{Pool: d.Pool, Config: tc, Transport: telegram.Mock{}, Chat: &chat}
	if _, e := tg.Connect(ctx, a, "mock:88001"); e != nil {
		t.Fatal(e)
	}
	update := func(id int64) telegram.Update {
		return telegram.Update{ID: id, Message: &telegram.Message{From: telegram.User{ID: 90001}, Chat: telegram.Chat{ID: 90001, Type: "private"}, Text: "Synthetic audit lookup"}}
	}
	if _, e := tg.Inject(ctx, a, update(1)); e != nil {
		t.Fatal(e)
	}
	if _, e := tg.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	var conv string
	if e := admin.QueryRow(ctx, `SELECT conversation_id FROM platform.telegram_principals WHERE organization_id=$1`, a.OrganizationID).Scan(&conv); e != nil {
		t.Fatal(e)
	}
	t.Run("handoff_cancels_queued_assistant", func(t *testing.T) {
		var pending int
		if e := admin.QueryRow(ctx, `SELECT count(*) FROM platform.telegram_outbox WHERE conversation_id=$1 AND status='pending'`, conv).Scan(&pending); e != nil || pending == 0 {
			t.Fatal("missing actual queued answer", e)
		}
		if _, e := chat.Control(ctx, a, conv, "human", 0); e != nil {
			t.Fatal(e)
		}
		if e := admin.QueryRow(ctx, `SELECT count(*) FROM platform.telegram_outbox WHERE conversation_id=$1 AND status IN ('pending','sending')`, conv).Scan(&pending); e != nil || pending != 0 {
			t.Fatal("automatic response survived handoff", e, pending)
		}
	})
	t.Run("disconnect_cancels_already_claimed_send", func(t *testing.T) {
		// Explicit staff response remains deliverable after handoff.
		if _, e := chat.Send(ctx, a, conv, conversations.SendInput{Text: "Synthetic staff", Role: "staff", IdempotencyKey: auth.UUID()}); e != nil {
			t.Fatal(e)
		}
		cap := auth.UUID()
		var raw []byte
		if e := d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim_send($1,$2,NULL,'mock')`, cap, tg.WorkerProof()).Scan(&raw); e != nil || string(raw) == "null" {
			t.Fatal("claim staff", e)
		}
		if _, e := tg.Disconnect(ctx, a); e != nil {
			t.Fatal(e)
		}
		var state string
		var job struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &job)
		if e := admin.QueryRow(ctx, `SELECT status FROM platform.telegram_outbox WHERE id=$1`, job.ID).Scan(&state); e != nil || state != "cancelled" {
			t.Fatal("claimed send remained active after disconnect", e, state)
		}
		var key *int64
		if e := d.Pool.QueryRow(ctx, `SELECT platform.telegram_prepare_send($1,$2,'mock',NULL)`, cap, tg.WorkerProof()).Scan(&key); e != nil || key != nil {
			t.Fatal("disconnected send admitted", e)
		}
	})
	t.Run("send_admission_serializes_disconnect", func(t *testing.T) {
		if _, e := tg.Connect(ctx, a, "mock:88001"); e != nil {
			t.Fatal(e)
		}
		if _, e := chat.Send(ctx, a, conv, conversations.SendInput{Text: "Synthetic staff two", Role: "staff", IdempotencyKey: auth.UUID()}); e != nil {
			t.Fatal(e)
		}
		cap := auth.UUID()
		var raw []byte
		if e := d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim_send($1,$2,NULL,'mock')`, cap, tg.WorkerProof()).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		conn, e := d.Pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer conn.Release()
		var key *int64
		if e = conn.QueryRow(ctx, `SELECT platform.telegram_prepare_send($1,$2,'mock',NULL)`, cap, tg.WorkerProof()).Scan(&key); e != nil || key == nil {
			t.Fatal("admission", e)
		}
		released := false
		defer func() {
			if !released {
				_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, *key)
			}
		}()
		done := make(chan error, 1)
		var started atomic.Bool
		go func() { started.Store(true); _, e := tg.Disconnect(ctx, a); done <- e }()
		for !started.Load() {
			time.Sleep(time.Millisecond)
		}
		observer, e := pgx.Connect(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"))
		if e != nil {
			t.Fatal(e)
		}
		defer observer.Close(context.Background())
		var idle bool
		if e = observer.QueryRow(ctx, `SELECT state='idle' AND xact_start IS NULL FROM pg_stat_activity WHERE pid=$1`, conn.Conn().PgConn().PID()).Scan(&idle); e != nil || !idle {
			t.Fatal("network admission kept a DB transaction open", e)
		}
		waiting := false
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if e = admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks waiting JOIN pg_locks held ON held.locktype=waiting.locktype AND held.classid=waiting.classid AND held.objid=waiting.objid WHERE waiting.locktype='advisory' AND NOT waiting.granted AND held.granted AND held.pid=$1)`, conn.Conn().PgConn().PID()).Scan(&waiting); e != nil {
				t.Fatal(e)
			}
			if waiting {
				break
			}
			select {
			case e := <-done:
				t.Fatal("disconnect skipped the send lock", e)
			default:
			}
			time.Sleep(time.Millisecond)
		}
		if !waiting {
			t.Fatal("no actual PostgreSQL lock wait observed")
		}

		// Finishing on this SAME pinned connection avoids pool-size=2 deadlock.
		if _, e = conn.Exec(ctx, `SELECT platform.telegram_finish_send($1,'sent',0,1)`, cap); e != nil {
			t.Fatal(e)
		}
		if _, e = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, *key); e != nil {
			t.Fatal(e)
		}
		released = true
		// Release before Disconnect.Status needs its next transaction.
		conn.Release()
		if e = <-done; e != nil {
			t.Fatal(e)
		}
	})
	t.Run("committed_message_crash_recovers_without_model_replay", func(t *testing.T) {
		raw, e := chat.Create(ctx, a)
		if e != nil {
			t.Fatal(e)
		}
		var c struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &c)
		key := auth.UUID()
		text := "Synthetic interrupted message"
		_, e = admin.Exec(ctx, `INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,client_key,request_hash) VALUES($1,$2,$3,'customer',$4,$1,$5)`, key, a.OrganizationID, c.ID, text, auth.Hash("customer:"+text))
		if e != nil {
			t.Fatal(e)
		}
		_, e = admin.Exec(ctx, `UPDATE platform.conversations SET message_count=1,in_flight=$2,lease_until=now()+interval '1 minute' WHERE id=$1`, c.ID, auth.UUID())
		if e != nil {
			t.Fatal(e)
		}
		input := conversations.SendInput{Text: text, IdempotencyKey: key}
		if _, e = chat.Send(ctx, a, c.ID, input); e == nil {
			t.Fatal("unfinished live lease incorrectly acknowledged as finished")
		}
		if _, e = admin.Exec(ctx, `UPDATE platform.conversations SET lease_until=now()-interval '1 second' WHERE id=$1`, c.ID); e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 2; i++ {
			if _, e = chat.Send(ctx, a, c.ID, input); e != nil {
				t.Fatal(e)
			}
		}
		var mode string
		var messages int
		if e = admin.QueryRow(ctx, `SELECT control_mode,(SELECT count(*) FROM platform.messages WHERE conversation_id=$1) FROM platform.conversations WHERE id=$1`, c.ID).Scan(&mode, &messages); e != nil || mode != "human" || messages != 2 {
			t.Fatal("interrupted recovery lost/duplicated notice", e, mode, messages)
		}
	})

	t.Run("budget_staff_cannot_change_owner_limits", func(t *testing.T) {
		if _, e := admin.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',true)`, a.OrganizationID, b.Session.User.ID); e != nil {
			t.Fatal(e)
		}
		defer func() {
			_, _ = admin.Exec(ctx, `DELETE FROM platform.organization_members WHERE organization_id=$1 AND user_id=$2`, a.OrganizationID, b.Session.User.ID)
		}()
		staff := b
		staff.OrganizationID = a.OrganizationID
		svc := costcontrol.Service{Pool: d.Pool, Config: cfg}
		_, e := svc.Update(ctx, staff, costcontrol.Change{Settings: costcontrol.Settings{AutoEnabled: true}, ExpectedVersion: 1})
		var ae *auth.Error
		if !errors.As(e, &ae) || ae.Status != 403 {
			t.Fatal("staff changed owner budget", e)
		}
		_, e = svc.Update(ctx, a, costcontrol.Change{Settings: costcontrol.Settings{AutoEnabled: true, RequestLimit: 1}, ExpectedVersion: 1})
		if !errors.As(e, &ae) || ae.Status != 422 {
			t.Fatal("owner bypassed zero platform cap", e)
		}
	})

	t.Run("channel_cannot_rewrite_message_actor_or_control_metadata", func(t *testing.T) {
		if _, e := tg.Connect(ctx, a, "mock:88001"); e != nil {
			t.Fatal(e)
		}
		u := update(77)
		if _, e := tg.Inject(ctx, a, u); e != nil {
			t.Fatal(e)
		}
		cap := auth.UUID()
		var raw []byte
		if e := d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim($1,$2,$3,$4,7,$5,NULL,'mock')`, cap, auth.UUID(), auth.UUID(), auth.UUID(), tg.WorkerProof()).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var job telegram.Job
		if e := json.Unmarshal(raw, &job); e != nil {
			t.Fatal(e)
		}
		channel := catalog.Tenant{OrganizationID: a.OrganizationID, ChannelCapability: cap}
		tx, e := tenancy.Begin(ctx, d.Pool, channel, true, true)
		if e != nil {
			t.Fatal(e)
		}
		id := auth.UUID()
		if _, e = tx.Exec(ctx, `INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,client_key) VALUES($1,$2,$3,'customer','synthetic',$1)`, id, a.OrganizationID, job.ConversationID); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		for _, change := range []string{"role='staff',actor_id=platform.current_user_id()", "data='{\"handoff_notice\":true}'::jsonb"} {
			tx, e = tenancy.Begin(ctx, d.Pool, channel, true, true)
			if e != nil {
				t.Fatal(e)
			}
			_, e = tx.Exec(ctx, `UPDATE platform.messages SET `+change+` WHERE id=$1`, id)
			_ = tx.Rollback(ctx)
			var sql interface{ SQLState() string }
			if !errors.As(e, &sql) || sql.SQLState() != "42501" {
				t.Fatal("customer channel rewrote actor/control metadata", e)
			}
		}
	})

	t.Run("out_of_order_ids_follow_durable_arrival_and_single_lease", func(t *testing.T) {
		tenant := owner()
		isolated := tg
		if _, e := isolated.Connect(ctx, tenant, "mock:88003"); e != nil {
			t.Fatal(e)
		}
		for _, id := range []int64{200, 100} {
			if _, e := isolated.Inject(ctx, tenant, update(id)); e != nil {
				t.Fatal(e)
			}
		}
		claim := func(cap string) []byte {
			var raw []byte
			if e := d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim($1,$2,$3,$4,7,$5,NULL,'mock')`, cap, auth.UUID(), auth.UUID(), auth.UUID(), isolated.WorkerProof()).Scan(&raw); e != nil {
				t.Fatal(e)
			}
			return raw
		}
		first := auth.UUID()
		raw := claim(first)
		var job telegram.Job
		if e := json.Unmarshal(raw, &job); e != nil || job.Payload.ID != 200 || job.OrganizationID != tenant.OrganizationID {
			t.Fatal("first durable arrival not claimed", e)
		}
		if raw = claim(auth.UUID()); len(raw) > 0 && string(raw) != "null" {
			t.Fatal("second message passed active lease")
		}
		if _, e := d.Pool.Exec(ctx, `SELECT platform.telegram_finish($1,NULL)`, first); e != nil {
			t.Fatal(e)
		}
		second := auth.UUID()
		raw = claim(second)
		if e := json.Unmarshal(raw, &job); e != nil || job.Payload.ID != 100 {
			t.Fatal("late numeric ID lost", e)
		}
		if _, e := d.Pool.Exec(ctx, `SELECT platform.telegram_finish($1,NULL)`, second); e != nil {
			t.Fatal(e)
		}
	})
	testAuditRLS(t, d, admin, ctx, b)
	testTelegramHTTPAudit(t, d, admin, ctx, owner())
	t.Run("readiness_fails_if_rls_is_disabled", func(t *testing.T) {
		if _, e := admin.Exec(ctx, `ALTER TABLE platform.messages DISABLE ROW LEVEL SECURITY`); e != nil {
			t.Fatal(e)
		}
		defer func() {
			_, _ = admin.Exec(context.Background(), `ALTER TABLE platform.messages ENABLE ROW LEVEL SECURITY`)
		}()
		if e := d.Ready(ctx); e == nil {
			t.Fatal("readiness reported secure with tenant RLS disabled")
		}
	})

}

func testAuditRLS(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context, outsider catalog.Tenant) {
	t.Helper()
	rows, e := admin.Query(ctx, `SELECT c.relname,EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attname='organization_id'),c.relrowsecurity,c.relforcerowsecurity,r.rolname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_roles r ON r.oid=c.relowner WHERE n.nspname='platform' AND c.relkind='r' AND (c.relname='organizations' OR EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attname='organization_id')) ORDER BY c.relname`)
	if e != nil {
		t.Fatal(e)
	}
	type table struct {
		name, owner     string
		org, rls, force bool
	}
	tables := []table{}
	for rows.Next() {
		var v table
		if e = rows.Scan(&v.name, &v.org, &v.rls, &v.force, &v.owner); e != nil {
			t.Fatal(e)
		}
		tables = append(tables, v)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	// Newly added tables must pass the same outsider and tenant-substitution probes.
	if len(tables) != 62 {
		t.Fatalf("tenant inventory changed: got %d; audit all new tables", len(tables))
	}
	// The general regression keeps conversational routing disabled to preserve its
	// baseline. Seed one bound context so every negative probe operates on a row.
	execTag, err := admin.Exec(ctx, `INSERT INTO platform.conversation_routes(organization_id,conversation_id,customer_id,channel)
	 SELECT organization_id,id,customer_id,channel FROM platform.conversations
	 WHERE organization_id<>$1 AND channel IN ('telegram','whatsapp','instagram')
	 ORDER BY created_at,id LIMIT 1 ON CONFLICT DO NOTHING`, outsider.OrganizationID)
	if err != nil || execTag.RowsAffected() != 1 {
		t.Fatalf("conversation routing RLS fixture: %v", err)
	}
	expectedTenantTables := map[string]bool{"instagram_connections": false, "instagram_oauth_states": false, "conversation_routes": false}
	for _, v := range tables {
		if _, exists := expectedTenantTables[v.name]; exists {
			expectedTenantTables[v.name] = true
		}
		t.Run("RLS_"+v.name, func(t *testing.T) {
			if !v.rls || !v.force || v.owner != "aba_migrator" {
				t.Fatal("unsafe RLS/owner")
			}
			column := "organization_id"
			if !v.org {
				column = "id"
			}
			var raw []byte
			var org string
			e := admin.QueryRow(ctx, fmt.Sprintf(`SELECT to_jsonb(x),%s::text FROM platform.%s x WHERE %s<>$1 LIMIT 1`, column, v.name, column), outsider.OrganizationID).Scan(&raw, &org)
			if e != nil {
				t.Fatal("no actual row for negative test", v.name, e)
			}
			// Probe a real inventory row by its composite primary key. The scale
			// fixture can have tens of thousands of rows in the same organization.
			var inventoryVariant string
			if v.name == "inventory" {
				var row struct {
					VariantID string `json:"variant_id"`
				}
				if e := json.Unmarshal(raw, &row); e != nil || row.VariantID == "" {
					t.Fatalf("missing inventory variant in source row: %v", e)
				}
				inventoryVariant = row.VariantID
			}
			for _, op := range []string{"read", "insert", "update", "delete"} {
				tx, e := d.Pool.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(outsider.Session.Hash), org)
				if e != nil {
					t.Fatal(e)
				}
				query := map[string]string{"read": fmt.Sprintf(`SELECT count(*) FROM platform.%s WHERE %s=$1`, v.name, column), "insert": fmt.Sprintf(`INSERT INTO platform.%s OVERRIDING SYSTEM VALUE SELECT * FROM jsonb_populate_record(NULL::platform.%s,$1::jsonb)`, v.name, v.name), "update": fmt.Sprintf(`UPDATE platform.%s SET %s=%s WHERE %s=$1`, v.name, column, column, column), "delete": fmt.Sprintf(`DELETE FROM platform.%s WHERE %s=$1`, v.name, column)}[op]
				args := []any{org}
				if op == "insert" {
					args[0] = raw
				} else if inventoryVariant != "" {
					query += " AND variant_id=$2"
					args = append(args, inventoryVariant)
				}
				if op == "read" {
					var count int
					e = tx.QueryRow(ctx, query, args...).Scan(&count)
					if e == nil && count != 0 {
						t.Error("foreign SELECT leaked")
					}
				} else {
					tag, err := tx.Exec(ctx, query, args...)
					e = err
					if e == nil && tag.RowsAffected() != 0 {
						t.Error("foreign write succeeded", op)
					}
				}
				_ = tx.Rollback(ctx)
				if e != nil {
					var sql interface{ SQLState() string }
					if !errors.As(e, &sql) || sql.SQLState() != "42501" {
						t.Fatal("not a demonstrated RLS/privilege denial", op, e)
					}
				}
			}

			// A legitimate owner still cannot re-parent its visible row to organization B.
			var ownerID string
			if e = admin.QueryRow(ctx, `SELECT user_id FROM platform.organization_members WHERE organization_id=$1 AND role='owner' LIMIT 1`, org).Scan(&ownerID); e != nil {
				t.Fatal(e)
			}
			hash := auth.Hash(auth.Token())
			if _, e = admin.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), ownerID, hash, auth.Hash(auth.Token())); e != nil {
				t.Fatal(e)
			}
			tx, e := d.Pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(hash), org)
			if e == nil {
				query := fmt.Sprintf(`UPDATE platform.%s SET %s=$1 WHERE %s=$2`, v.name, column, column)
				args := []any{outsider.OrganizationID, org}
				if inventoryVariant != "" {
					query += " AND variant_id=$3"
					args = append(args, inventoryVariant)
				}
				_, e = tx.Exec(ctx, query, args...)
			}
			_ = tx.Rollback(ctx)
			var denied interface{ SQLState() string }
			if !errors.As(e, &denied) || denied.SQLState() != "42501" {
				t.Fatal("WITH CHECK/privilege did not prevent tenant substitution", e)
			}
		})
	}
	for name, present := range expectedTenantTables {
		if !present {
			t.Errorf("required tenant table missing from audit: %s", name)
		}
	}

	if dir := os.Getenv("ABA_AUDIT_EVIDENCE_DIR"); dir != "" {
		var inventory []byte
		e = admin.QueryRow(ctx, `SELECT jsonb_build_object('tables',(SELECT jsonb_agg(jsonb_build_object('table',c.relname,'rls',c.relrowsecurity,'force',c.relforcerowsecurity,'owner',pg_get_userbyid(c.relowner),'policies',(SELECT jsonb_agg(jsonb_build_object('name',p.polname,'command',p.polcmd,'roles',(SELECT jsonb_agg(CASE WHEN role_oid=0 THEN 'PUBLIC' ELSE pg_get_userbyid(role_oid)::text END) FROM unnest(p.polroles) role_oid),'using',pg_get_expr(p.polqual,p.polrelid),'with_check',pg_get_expr(p.polwithcheck,p.polrelid))) FROM pg_policy p WHERE p.polrelid=c.oid),'foreign_keys',(SELECT jsonb_agg(pg_get_constraintdef(k.oid)) FROM pg_constraint k WHERE k.conrelid=c.oid AND k.contype='f'),'runtime_privileges',jsonb_build_object('insert',has_table_privilege('aba_app',c.oid,'INSERT'),'update',has_table_privilege('aba_app',c.oid,'UPDATE'),'delete',has_table_privilege('aba_app',c.oid,'DELETE')))) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='platform' AND c.relkind='r'))`).Scan(&inventory)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, "rls_inventory.json"), inventory, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if !t.Failed() {
		t.Logf("VERIFIED: %d tenant tables ENABLE/FORCE/migration owner and outsider SELECT/INSERT/UPDATE/DELETE under aba_app", len(tables))
	}
}
