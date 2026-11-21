package catalog

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"strings"
)

// Health is deterministic over one database snapshot. Empty catalogues have no score.
type HealthInput struct{ Total, Complete, Priced, Known, Fresh, Conflicts, ImportErrors, ImportRows int }

func Health(h HealthInput) *int {
	if h.Total == 0 {
		return nil
	}
	ratio := func(n, d int) int {
		if d == 0 {
			return 100
		}
		if n < 0 {
			n = 0
		}
		if n > d {
			n = d
		}
		return n * 100 / d
	}
	n := (20*ratio(h.Complete, h.Total) + 20*ratio(h.Priced, h.Total) + 15*ratio(h.Known, h.Total) + 25*ratio(h.Fresh, h.Total) + 10*ratio(h.Total-h.Conflicts, h.Total) + 10*ratio(h.ImportRows-h.ImportErrors, h.ImportRows)) / 100
	return &n
}

// The inventory PK guarantees at most one row per variant. Keep its lookup
// parameterized: a generic plan cached before a bulk insert can otherwise scan
// the whole tenant inventory for each variant using stale one-row estimates.
const controlRows = `SELECT p.id product_id,p.name,p.status,p.description,p.brand,p.category_id,p.version product_version,p.catalog_source product_source,
 v.id variant_id,v.sku,v.variant_name,v.attributes,v.price_amount,v.currency_code,v.version variant_version,v.catalog_source price_source,v.price_updated_at,
 i.quantity_on_hand,i.quantity_reserved,i.quantity_on_hand-i.quantity_reserved available,i.version inventory_version,i.catalog_source stock_source,i.manual_updated_at stock_updated_at,
 platform.inventory_freshness_at(i.manual_updated_at,
 coalesce((SELECT fresh_hours FROM platform.catalog_policies WHERE organization_id=current_setting('app.organization_id')::uuid),24),
 coalesce((SELECT stale_hours FROM platform.catalog_policies WHERE organization_id=current_setting('app.organization_id')::uuid),72)) freshness
 FROM platform.products p JOIN platform.product_variants v ON (v.organization_id,v.product_id)=(p.organization_id,p.id)
 JOIN LATERAL (SELECT i.* FROM platform.inventory i WHERE (i.organization_id,i.variant_id)=(v.organization_id,v.id) LIMIT 1) i ON true
 WHERE p.status<>'archived' AND p.organization_id=current_setting('app.organization_id')::uuid`

// Control preserves the legacy no-query contract. New callers use ControlPage.
func (s Service) Control(ctx context.Context, t Tenant) (json.RawMessage, error) {
	return s.ControlPage(ctx, t, ControlQuery{legacy: true})
}

type ControlQuery struct {
	Page, ImportPage int
	Query, Filter    string
	legacy           bool
}

func (q *ControlQuery) Validate() error {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.ImportPage == 0 {
		q.ImportPage = 1
	}
	if q.Page < 1 || q.Page > 10000 || q.ImportPage < 1 || q.ImportPage > 10000 || !textValid(q.Query, 0, 200) {
		return invalid("Некорректная пагинация или поиск")
	}
	switch q.Filter {
	case "", "ALL", "FRESH", "AGING", "STALE", "UNKNOWN", "LOW", "PRICE", "INCOMPLETE":
	default:
		return invalid("Некорректный фильтр")
	}
	return nil
}

func (s Service) ControlPage(ctx context.Context, t Tenant, q ControlQuery) (json.RawMessage, error) {
	if e := q.Validate(); e != nil {
		return nil, e
	}
	tx, e := s.beginOwnerRead(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	// LLVM compilation of the RLS-heavy aggregate can consume its one-second
	// statement deadline under contention. This interactive transaction cannot
	// amortize that work; SET LOCAL leaves the pooled connection default intact.
	if _, e = tx.Exec(ctx, "SET LOCAL jit=off"); e != nil {
		return nil, dbError(e)
	}
	var h HealthInput
	// Only aggregates cross the database boundary, independent of catalogue size.
	var metrics []byte
	e = tx.QueryRow(ctx, `WITH r AS (`+controlRows+`), a AS (SELECT
 count(*) total,count(*) FILTER(WHERE description<>'' AND brand<>'' AND attributes<>'{}'::jsonb) complete,
 count(*) FILTER(WHERE price_amount>0) priced,count(*) FILTER(WHERE stock_updated_at IS NOT NULL AND freshness<>'UNKNOWN') known,
 count(*) FILTER(WHERE freshness='FRESH') fresh FROM r)
 SELECT total,complete,priced,known,fresh,jsonb_build_object(
 'products',(SELECT count(DISTINCT product_id) FROM r),'active',(SELECT count(DISTINCT product_id) FROM r WHERE status='active'),
 'available',(SELECT count(DISTINCT product_id) FROM r WHERE status='active' AND freshness='FRESH' AND available>0),
 'low_stock',(SELECT count(DISTINCT product_id) FROM r WHERE freshness='FRESH' AND available<=coalesce((SELECT low_stock FROM platform.catalog_policies WHERE organization_id=$1),3)),
 'review',(SELECT count(DISTINCT product_id) FROM r WHERE freshness<>'FRESH'),'variants',total,'missing_price',total-priced,'incomplete',total-complete,
 'sources',jsonb_build_object('MANUAL',(SELECT count(*) FROM r WHERE stock_source='MANUAL'),'EXCEL',(SELECT count(*) FROM r WHERE stock_source='EXCEL'),'ONE_C',(SELECT count(*) FROM r WHERE stock_source='ONE_C')),
 'freshness',jsonb_build_object('FRESH',fresh,'AGING',(SELECT count(*) FROM r WHERE freshness='AGING'),'STALE',(SELECT count(*) FROM r WHERE freshness='STALE'),'UNKNOWN',(SELECT count(*) FROM r WHERE freshness='UNKNOWN'))) FROM a`, t.OrganizationID).Scan(&h.Total, &h.Complete, &h.Priced, &h.Known, &h.Fresh, &metrics)
	if e != nil {
		return nil, dbError(e)
	}
	if q.legacy && h.Total > 5000 {
		return nil, auth.Failure(422, "CATALOG_LIMIT", "Используйте постраничный каталог для более5000 вариантов")
	}
	size := 50
	if q.legacy {
		size = 5000
	}
	query := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q.Query)
	pageSQL := `WITH r AS (` + controlRows + `), matched AS (SELECT * FROM r WHERE
 ($1='' OR name ILIKE '%'||$1||'%' OR sku ILIKE '%'||$1||'%') AND
 ($2 IN ('','ALL') OR freshness=$2 OR ($2='PRICE' AND price_amount<=0) OR ($2='INCOMPLETE' AND (brand='' OR description='' OR attributes='{}'::jsonb)) OR
 ($2='LOW' AND freshness='FRESH' AND available<=coalesce((SELECT low_stock FROM platform.catalog_policies WHERE organization_id=$3),3))))
 SELECT jsonb_build_object('items',coalesce((SELECT jsonb_agg(x) FROM (SELECT * FROM matched ORDER BY name,sku,variant_id LIMIT $5 OFFSET $4) x),'[]'),'total',(SELECT count(*) FROM matched))`
	rows, e := object(ctx, tx, pageSQL, query, q.Filter, t.OrganizationID, (q.Page-1)*size, size)
	if e != nil {
		return nil, e
	}
	var issues []byte
	e = tx.QueryRow(ctx, `SELECT coalesce(jsonb_agg(x),'[]') FROM (SELECT id,status,preview,created_at FROM platform.catalog_imports WHERE minimized_at IS NULL ORDER BY created_at DESC LIMIT 1) x`).Scan(&issues)
	if e != nil {
		return nil, dbError(e)
	}
	var last []struct {
		Preview ImportPreview `json:"preview"`
	}
	_ = json.Unmarshal(issues, &last)
	if len(last) > 0 {
		h.ImportRows = last[0].Preview.Total
		h.ImportErrors = last[0].Preview.Errors + last[0].Preview.Duplicates
		h.Conflicts = last[0].Preview.Conflicts
		if h.ImportRows == 0 && h.ImportErrors > 0 {
			h.ImportRows = h.ImportErrors
		}
	}
	var metricMap map[string]any
	if json.Unmarshal(metrics, &metricMap) != nil {
		return nil, auth.Unavailable()
	}
	metricMap["conflicts"] = h.Conflicts
	metricMap["import_errors"] = h.ImportErrors
	metrics, e = json.Marshal(metricMap)
	if e != nil {
		return nil, auth.Unavailable()
	}
	score := Health(h)
	b, _ := json.Marshal(score)
	result, e := object(ctx, tx, `SELECT jsonb_build_object('rows',$1::jsonb->'items','rows_total',$1::jsonb->'total','page',$6::integer,'page_size',$9::integer,'import_page',$7::integer,'import_page_size',100,'imports_total',(SELECT count(*) FROM platform.catalog_imports),'health_score',$2::jsonb,'health_version','catalog-health-v1','evaluated_at',now(),
 'store_name',(SELECT name FROM platform.organizations WHERE id=$3),
 'policy',coalesce((SELECT to_jsonb(c)-'cleanup_hash' FROM platform.catalog_policies c WHERE organization_id=$3),jsonb_build_object('fresh_hours',24,'stale_hours',72,'low_stock',3,'preview_days',1,'error_days',7,'change_days',90,'version',0)),
 'last_successful_import',(SELECT max(applied_at) FROM platform.catalog_imports WHERE status='APPLIED'),
 'last_stock_check',(SELECT max(manual_updated_at) FROM platform.inventory WHERE organization_id=$3),
 'imports',coalesce((SELECT jsonb_agg(x) FROM (SELECT id,file_name,status,version,created_by,created_at,applied_at,expires_at,retention_hold,minimized_at,CASE WHEN summary='{}' THEN preview-'lines' ELSE summary END summary FROM platform.catalog_imports ORDER BY created_at DESC,id DESC LIMIT 100 OFFSET $8) x),'[]'),
 'source_history',coalesce((SELECT jsonb_agg(x) FROM (SELECT * FROM platform.catalog_source_changes ORDER BY created_at DESC LIMIT 100) x),'[]'),
 'latest_import',$4::jsonb,'metrics',$5::jsonb)`, rows, string(b), t.OrganizationID, string(issues), string(metrics), q.Page, q.ImportPage, (q.ImportPage-1)*100, size)
	return commit(ctx, tx, result, e)
}

type SourceChange struct {
	OperationID string `json:"operation_id"`
	ProductID   string `json:"product_id"`
	Target      string `json:"target"`
	Reason      string `json:"reason"`
	Confirm     bool   `json:"confirm"`
}

func sourceSnapshot(ctx context.Context, tx pgx.Tx, id string) (json.RawMessage, error) {
	return object(ctx, tx, `SELECT jsonb_build_object('product',to_jsonb(p),'variants',(SELECT jsonb_agg(to_jsonb(v) ORDER BY v.id) FROM platform.product_variants v WHERE v.product_id=p.id),'inventory',(SELECT jsonb_agg(to_jsonb(i) ORDER BY i.variant_id) FROM platform.inventory i JOIN platform.product_variants v ON v.id=i.variant_id WHERE v.product_id=p.id)) FROM platform.products p WHERE p.id=$1 AND p.status<>'archived'`, id)
}
func (s Service) ChangeSource(ctx context.Context, t Tenant, in SourceChange) (json.RawMessage, error) {
	if !organizations.ValidID(in.OperationID) || !organizations.ValidID(in.ProductID) || (in.Target != "MANUAL" && in.Target != "EXCEL") || !textValid(in.Reason, 1, 500) {
		return nil, invalid("Укажите товар, MANUAL/EXCEL, UUID операции и основание")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if e = lockCatalog(ctx, tx); e != nil {
		return nil, e
	}
	var state, target, product, reason string
	var snapshot []byte
	var expired bool
	e = tx.QueryRow(ctx, `SELECT status,target,product_id::text,reason,snapshot,expires_at<=now() FROM platform.catalog_source_changes WHERE id=$1 FOR UPDATE`, in.OperationID).Scan(&state, &target, &product, &reason, &snapshot, &expired)
	if e == pgx.ErrNoRows {
		if in.Confirm {
			return nil, conflict()
		}
		var count int
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM platform.catalog_source_changes WHERE status='PREVIEW' AND expires_at>now()`).Scan(&count)
		if count >= 100 {
			return nil, invalid("Слишком много активных предпросмотров")
		}
		raw, err := sourceSnapshot(ctx, tx, in.ProductID)
		if err != nil {
			return nil, err
		}
		_, e = tx.Exec(ctx, `INSERT INTO platform.catalog_source_changes(id,organization_id,product_id,target,reason,snapshot,actor_id) VALUES($1,$2,$3,$4,$5,$6,platform.current_user_id())`, in.OperationID, t.OrganizationID, in.ProductID, in.Target, in.Reason, raw)
		if e != nil {
			return nil, dbError(e)
		}
	} else if e != nil {
		return nil, dbError(e)
	} else {
		if target != in.Target || product != in.ProductID || reason != in.Reason {
			return nil, conflict()
		}
		if in.Confirm && state != "APPLIED" {
			if expired {
				return nil, conflict()
			}
			raw, err := sourceSnapshot(ctx, tx, in.ProductID)
			if err != nil {
				return nil, err
			}
			var same bool
			if e = tx.QueryRow(ctx, `SELECT $1::jsonb=$2::jsonb`, string(raw), string(snapshot)).Scan(&same); e != nil || !same {
				return nil, conflict()
			}
			_, e = tx.Exec(ctx, `SELECT set_config('app.catalog_handoff','true',true)`)
			if e != nil {
				return nil, dbError(e)
			}
			for _, q := range []string{`UPDATE platform.products SET catalog_source=$2,version=version+1,updated_at=now() WHERE id=$1`, `UPDATE platform.product_variants SET catalog_source=$2,version=version+1,updated_at=now() WHERE product_id=$1`, `UPDATE platform.inventory SET catalog_source=$2,version=version+1,updated_at=now() WHERE variant_id IN(SELECT id FROM platform.product_variants WHERE product_id=$1)`} {
				if _, e = tx.Exec(ctx, q, in.ProductID, in.Target); e != nil {
					return nil, dbError(e)
				}
			}
			_, e = tx.Exec(ctx, `UPDATE platform.catalog_source_changes SET status='APPLIED',applied_at=now() WHERE id=$1`, in.OperationID)
			if e != nil {
				return nil, dbError(e)
			}
			if _, e = tx.Exec(ctx, `INSERT INTO platform.catalog_events(organization_id,actor_id,resource_id,kind) VALUES($1,platform.current_user_id(),$2,'SOURCE_CHANGED')`, t.OrganizationID, in.OperationID); e != nil {
				return nil, e
			}
		}
	}
	raw, e := object(ctx, tx, `SELECT to_jsonb(x) FROM platform.catalog_source_changes x WHERE id=$1`, in.OperationID)
	return commit(ctx, tx, raw, e)
}

type Policy struct {
	FreshHours  int   `json:"fresh_hours"`
	StaleHours  int   `json:"stale_hours"`
	LowStock    int   `json:"low_stock"`
	PreviewDays int   `json:"preview_days"`
	ErrorDays   int   `json:"error_days"`
	ChangeDays  int   `json:"change_days"`
	Version     int64 `json:"expected_version"`
}

func (s Service) SavePolicy(ctx context.Context, t Tenant, p Policy) (json.RawMessage, error) {
	if p.FreshHours < 1 || p.FreshHours > 720 || p.StaleHours < p.FreshHours || p.StaleHours > 2160 || p.LowStock < 0 || p.LowStock > 1000000 || p.PreviewDays < 1 || p.PreviewDays > 30 || p.ErrorDays < p.PreviewDays || p.ErrorDays > 365 || p.ChangeDays < 30 || p.ChangeDays > 3650 {
		return nil, invalid("Проверьте пороги актуальности и сроки хранения")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var v int64
	e = tx.QueryRow(ctx, `SELECT version FROM platform.catalog_policies WHERE organization_id=$1`, t.OrganizationID).Scan(&v)
	if e != nil && e != pgx.ErrNoRows {
		return nil, dbError(e)
	}
	if v != p.Version {
		return nil, conflict()
	}
	raw, e := object(ctx, tx, `INSERT INTO platform.catalog_policies(organization_id,fresh_hours,stale_hours,low_stock,preview_days,error_days,change_days) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(organization_id) DO UPDATE SET fresh_hours=$2,stale_hours=$3,low_stock=$4,preview_days=$5,error_days=$6,change_days=$7,version=catalog_policies.version+1 RETURNING to_jsonb(catalog_policies)-'cleanup_hash'`, t.OrganizationID, p.FreshHours, p.StaleHours, p.LowStock, p.PreviewDays, p.ErrorDays, p.ChangeDays)
	if e != nil {
		return nil, e
	}
	if e = s.bindCleanup(ctx, tx, t); e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, t, "", "CATALOG_POLICY_UPDATED"); e != nil {
		return nil, e
	}
	return commit(ctx, tx, raw, nil)
}

// Cleanup uses a bounded capability-scoped transaction; no filesystem deletion or audit deletion.
func (s Service) Cleanup(ctx context.Context, proof []byte) (int, error) {
	if len(proof) != 32 {
		return 0, auth.Failure(403, "FORBIDDEN", "Worker capability required")
	}
	rows, e := s.Pool.Query(ctx, `SELECT platform.catalog_cleanup_orgs($1)`, proof)
	if e != nil {
		return 0, dbError(e)
	}
	var orgs []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return 0, e
		}
		orgs = append(orgs, id)
	}
	rows.Close()
	if rows.Err() != nil {
		return 0, rows.Err()
	}
	n := 0
	for _, org := range orgs {
		tx, e := s.Pool.Begin(ctx)
		if e != nil {
			return n, e
		}
		_, e = tx.Exec(ctx, `SELECT set_config('app.organization_id',$1,true),set_config('app.catalog_cleanup_proof',$2,true)`, org, hex.EncodeToString(proof))
		if e == nil {
			var count int
			e = tx.QueryRow(ctx, `WITH candidates AS (SELECT j.id FROM platform.catalog_imports j LEFT JOIN platform.catalog_policies p ON p.organization_id=j.organization_id WHERE j.organization_id=$1 AND NOT j.retention_hold AND (
 (j.status IN ('UPLOADED','PREVIEW') AND j.expires_at<=now() AND j.created_at<now()-make_interval(days=>coalesce(p.preview_days,1))) OR
 (j.status IN ('CANCELLED','EXPIRED','FAILED') AND (j.minimized_at IS NULL OR j.preview<>'{}'::jsonb) AND j.created_at<now()-make_interval(days=>coalesce(p.error_days,7))) OR
 (j.status='APPLIED' AND j.minimized_at IS NULL AND coalesce(j.applied_at,j.created_at)<now()-make_interval(days=>coalesce(p.change_days,90)))) ORDER BY j.created_at LIMIT 20 FOR UPDATE OF j SKIP LOCKED), changed AS (
 UPDATE platform.catalog_imports j SET status=CASE WHEN status IN ('UPLOADED','PREVIEW') THEN 'EXPIRED' ELSE status END,summary=CASE WHEN summary='{}' THEN preview-'lines' ELSE summary END,grid='[]',mapping='{}',preview=CASE WHEN j.status IN ('UPLOADED','PREVIEW') AND j.created_at>now()-make_interval(days=>coalesce((SELECT error_days FROM platform.catalog_policies WHERE organization_id=j.organization_id),7)) THEN j.preview ELSE '{}'::jsonb END,file_name='catalog.xlsx',minimized_at=now(),version=version+1 FROM candidates c WHERE j.id=c.id RETURNING j.id,j.organization_id),events AS(INSERT INTO platform.catalog_events(organization_id,import_id,kind) SELECT organization_id,id,'IMPORT_RETENTION_MINIMIZED' FROM changed) SELECT count(*) FROM changed`, org).Scan(&count)
			if e == nil {
				n += count
			}
		}
		if e != nil {
			_ = tx.Rollback(ctx)
			return n, dbError(e)
		}
		if e = tx.Commit(ctx); e != nil {
			return n, dbError(e)
		}
	}
	return n, nil
}

func (s Service) ImportHold(ctx context.Context, t Tenant, id string, version int64, hold bool) (json.RawMessage, error) {
	if !organizations.ValidID(id) || version < 1 {
		return nil, invalid("Некорректная операция")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	raw, e := object(ctx, tx, `UPDATE platform.catalog_imports SET retention_hold=$3,version=version+1 WHERE id=$1 AND version=$2 RETURNING to_jsonb(catalog_imports)-'grid'-'preview'`, id, version, hold)
	if e != nil {
		return nil, conflict()
	}
	if e = audit(ctx, tx, t, id, "IMPORT_RETENTION_HOLD_UPDATED"); e != nil {
		return nil, e
	}
	return commit(ctx, tx, raw, nil)
}

// WorkerProof is domain separated from the local auth key and never returned to a client.
func WorkerProof(key []byte) []byte {
	if len(key) != 32 {
		return nil
	}
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte("aba:catalog:retention-worker:v1"))
	return h.Sum(nil)
}
func (s Service) bindCleanup(ctx context.Context, tx pgx.Tx, t Tenant) error {
	if len(s.CleanupProof) != 32 {
		return nil
	}
	tag, e := tx.Exec(ctx, `INSERT INTO platform.catalog_policies(organization_id,cleanup_hash) VALUES($1,sha256($2)) ON CONFLICT(organization_id) DO UPDATE SET cleanup_hash=excluded.cleanup_hash WHERE catalog_policies.cleanup_hash IS DISTINCT FROM excluded.cleanup_hash`, t.OrganizationID, s.CleanupProof)
	if e != nil {
		return dbError(e)
	}
	if tag.RowsAffected() > 0 {
		return audit(ctx, tx, t, "", "CATALOG_CLEANUP_BOUND")
	}
	return nil
}

// RebindCleanup explicitly binds only the selected owner's tenant to the current
// backend proof. Optimistic fencing prevents stale or concurrent confirmations.
func (s Service) RebindCleanup(ctx context.Context, t Tenant, version int64, confirm bool) (json.RawMessage, error) {
	if !confirm || version < 0 {
		return nil, invalid("Подтвердите перепривязку capability")
	}
	if len(s.CleanupProof) != 32 {
		return nil, auth.Unavailable()
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var current int64
	e = tx.QueryRow(ctx, `SELECT version FROM platform.catalog_policies WHERE organization_id=$1 FOR UPDATE`, t.OrganizationID).Scan(&current)
	if e != nil && e != pgx.ErrNoRows {
		return nil, dbError(e)
	}
	if current != version {
		return nil, conflict()
	}
	if e = s.bindCleanup(ctx, tx, t); e != nil {
		return nil, e
	}
	raw, e := object(ctx, tx, `UPDATE platform.catalog_policies SET version=version+1 WHERE organization_id=$1 RETURNING jsonb_build_object('version',version,'bound',true)`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, t, "", "CATALOG_CLEANUP_REBIND_CONFIRMED"); e != nil {
		return nil, e
	}
	return commit(ctx, tx, raw, nil)
}
