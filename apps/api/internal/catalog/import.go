package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
)

type ImportLine struct {
	Row      int    `json:"row"`
	Input    Input  `json:"input"`
	Category string `json:"category"`
	State    string `json:"state"`
	Reason   string `json:"reason"`
	OldPrice *int64 `json:"old_price"`
	OldStock *int64 `json:"old_stock"`
}
type ImportPreview struct {
	Total      int          `json:"total"`
	New        int          `json:"new"`
	Updated    int          `json:"updated"`
	Unchanged  int          `json:"unchanged"`
	Errors     int          `json:"errors"`
	Conflicts  int          `json:"conflicts"`
	Duplicates int          `json:"duplicates"`
	Lines      []ImportLine `json:"lines"`
}

var decimalPrice = regexp.MustCompile(`^[0-9]{1,11}([.,][0-9]{1,2})?$`)

func parsePrice(s string) (int64, error) {
	if !decimalPrice.MatchString(s) {
		return 0, invalid("Цена: рубли, до двух знаков после запятой")
	}
	parts := strings.Split(strings.ReplaceAll(s, ",", "."), ".")
	a, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil {
		return 0, invalid("Цена")
	}
	n := a * 100
	if len(parts) == 2 {
		p := parts[1]
		if len(p) == 1 {
			p += "0"
		}
		b, _ := strconv.ParseInt(p, 10, 64)
		n += b
	}
	return n, nil
}

var importFields = map[string]bool{"sku": true, "name": true, "category": true, "brand": true, "model": true, "description": true, "price": true, "currency": true, "quantity": true, "availability": true, "attributes": true}

func normalize(grid Grid, m map[string]int) ([]ImportLine, error) {
	if len(grid) < 2 {
		return nil, invalid("Пустой лист")
	}
	seenCols := map[int]bool{}
	for k, v := range m {
		if !importFields[k] || v < 0 || v >= len(grid[0]) || seenCols[v] {
			return nil, invalid("Некорректное или повторное сопоставление колонок")
		}
		seenCols[v] = true
	}
	for _, k := range []string{"sku", "name", "price", "quantity"} {
		if _, ok := m[k]; !ok {
			return nil, invalid("Обязательные колонки: SKU, название, цена, количество")
		}
	}
	lines := []ImportLine{}
	skuCount := map[string]int{}
	for n, row := range grid[1:] {
		get := func(k string) string {
			if i, ok := m[k]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		line := ImportLine{Row: n + 2, State: "VALID", Category: get("category")}
		in := Input{SKU: get("sku"), Name: get("name"), Brand: get("brand"), Description: get("description"), CurrencyCode: get("currency"), Attributes: map[string]string{}}
		var e error
		in.PriceAmount, e = parsePrice(get("price"))
		if e != nil {
			line.Reason = "INVALID_PRICE"
		}
		q, e := strconv.ParseInt(get("quantity"), 10, 64)
		in.QuantityOnHand = q
		if e != nil || q < 0 || q > 1000000000 {
			line.Reason = "INVALID_STOCK"
		}
		if a := get("attributes"); a != "" {
			if len(a) > 4096 || json.Unmarshal([]byte(a), &in.Attributes) != nil || in.Attributes == nil {
				line.Reason = "INVALID_ATTRIBUTES"
			}
		}
		if in.Attributes == nil {
			in.Attributes = map[string]string{}
		}
		if model := get("model"); model != "" {
			if old, ok := in.Attributes["model"]; ok && old != model {
				line.Reason = "MODEL_CONFLICT"
			}
			in.Attributes["model"] = model
		}
		if a := strings.ToLower(get("availability")); a != "" {
			available := a == "true" || a == "1" || a == "да" || a == "in_stock"
			unavailable := a == "false" || a == "0" || a == "нет" || a == "out_of_stock"
			if (!available && !unavailable) || available != (q > 0) {
				line.Reason = "AVAILABILITY_CONFLICT"
			}
		}
		if e = in.validateProduct(); e != nil {
			line.Reason = "INVALID_PRODUCT"
		}
		if e = in.validateVariant(); e != nil {
			line.Reason = "INVALID_VARIANT"
		}
		if !textValid(line.Category, 0, 120) {
			line.Reason = "INVALID_CATEGORY"
		}
		line.Input = in
		skuCount[in.SKU]++
		lines = append(lines, line)
	}
	for i := range lines {
		if skuCount[lines[i].Input.SKU] > 1 {
			lines[i].State = "DUPLICATE"
			lines[i].Reason = "DUPLICATE_SKU"
		} else if lines[i].Reason != "" {
			lines[i].State = "ERROR"
		}
	}
	return lines, nil
}
func fingerprint(ctx context.Context, tx pgx.Tx) (string, error) {
	var f string
	e := tx.QueryRow(ctx, `SELECT md5(coalesce((SELECT jsonb_agg(to_jsonb(p) ORDER BY id)::text FROM platform.products p WHERE organization_id=current_setting('app.organization_id')::uuid),'')||coalesce((SELECT jsonb_agg(to_jsonb(v) ORDER BY id)::text FROM platform.product_variants v WHERE organization_id=current_setting('app.organization_id')::uuid),'')||coalesce((SELECT jsonb_agg(to_jsonb(i) ORDER BY variant_id)::text FROM platform.inventory i WHERE organization_id=current_setting('app.organization_id')::uuid),'')||coalesce((SELECT jsonb_agg(to_jsonb(c) ORDER BY id)::text FROM platform.product_categories c WHERE organization_id=current_setting('app.organization_id')::uuid),''))`).Scan(&f)
	return f, e
}
func lockCatalog(ctx context.Context, tx pgx.Tx) error {
	for _, q := range []string{`SELECT id FROM platform.products WHERE organization_id=current_setting('app.organization_id')::uuid ORDER BY id FOR UPDATE`, `SELECT variant_id FROM platform.inventory WHERE organization_id=current_setting('app.organization_id')::uuid ORDER BY variant_id FOR UPDATE`} {
		r, e := tx.Query(ctx, q)
		if e != nil {
			return e
		}
		for r.Next() {
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return e
		}
	}
	return nil
}
func (s Service) UploadXLSX(ctx context.Context, t Tenant, key string, b []byte, names ...string) (json.RawMessage, error) {
	if !organizations.ValidID(key) {
		return nil, invalid("Нужен ключ идемпотентности UUID")
	}
	// Authorize before parsing user bytes; no unauthorized ZIP CPU/memory work.
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if e = s.bindCleanup(ctx, tx, t); e != nil {
		return nil, e
	}
	hash := sha256.Sum256(b)
	var id string
	var old []byte
	e = tx.QueryRow(ctx, `SELECT id,file_digest FROM platform.catalog_imports WHERE organization_id=$1 AND request_key=$2`, t.OrganizationID, key).Scan(&id, &old)
	if e == nil {
		if string(old) != string(hash[:]) {
			return nil, conflict()
		}
		return importObject(ctx, tx, id)
	}
	if e != pgx.ErrNoRows {
		return nil, dbError(e)
	}
	var count int
	var retained int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.catalog_imports WHERE organization_id=$1 AND minimized_at IS NULL`, t.OrganizationID).Scan(&retained); e != nil {
		return nil, dbError(e)
	}
	if retained >= 100 {
		return nil, auth.Failure(409, "IMPORT_HISTORY_LIMIT", "Лимит истории100 импортов; требуется контролируемая очистка")
	}
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.catalog_imports WHERE organization_id=$1 AND status IN ('UPLOADED','PREVIEW')`, t.OrganizationID).Scan(&count); e != nil {
		return nil, dbError(e)
	}
	if count >= 20 {
		return nil, auth.Failure(409, "IMPORT_LIMIT", "Отмените незавершённые импорты; максимум20")
	}
	grid, e := ReadXLSX(ctx, b)
	if e != nil {
		id = auth.UUID()
		_, dbErr := tx.Exec(ctx, `INSERT INTO platform.catalog_imports(id,organization_id,request_key,file_digest,grid,preview,summary,status,created_by) VALUES($1,$2,$3,$4,'[]','{"total":0,"errors":1}','{"total":0,"errors":1}','FAILED',platform.current_user_id())`, id, t.OrganizationID, key, hash[:])
		if dbErr != nil {
			return nil, dbError(dbErr)
		}
		return rejectedImport(ctx, tx, t, id, "IMPORT_FAILED", invalid(ErrWorkbook.Error()))
	}
	raw, _ := json.Marshal(grid)
	id = auth.UUID()
	filename := "catalog.xlsx"
	if len(names) > 0 && textValid(names[0], 1, 120) && !strings.ContainsAny(names[0], "/\\\r\n") {
		filename = names[0]
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform.catalog_imports(id,organization_id,request_key,file_digest,grid,created_by,file_name) VALUES($1,$2,$3,$4,$5,platform.current_user_id(),$6)`, id, t.OrganizationID, key, hash[:], raw, filename)
	if e != nil {
		return nil, dbError(e)
	}
	if e = audit(ctx, tx, t, id, "IMPORT_UPLOADED"); e != nil {
		return nil, e
	}
	data, e := importObject(ctx, tx, id)
	return commit(ctx, tx, data, e)
}
func importObject(ctx context.Context, tx pgx.Tx, id string) (json.RawMessage, error) {
	return object(ctx, tx, `SELECT jsonb_build_object('id',id,'status',status,'version',version,'headers',grid->0,'preview',preview,'mapping',mapping,'expires_at',expires_at,'minimized_at',minimized_at,'summary',summary,'retention_hold',retention_hold,'events',coalesce((SELECT jsonb_agg(to_jsonb(e) ORDER BY e.created_at,e.id) FROM platform.catalog_events e WHERE e.import_id=catalog_imports.id),'[]')) FROM platform.catalog_imports WHERE id=$1`, id)
}
func (s Service) ImportResult(ctx context.Context, t Tenant, id string) (json.RawMessage, error) {
	if !organizations.ValidID(id) {
		return nil, missing()
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	return importObject(ctx, tx, id)
}
func (s Service) MapImport(ctx context.Context, t Tenant, id string, version int64, m map[string]int) (json.RawMessage, error) {
	if !organizations.ValidID(id) || version < 1 {
		return nil, invalid("Некорректный импорт/версия")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	var state string
	var v int64
	var expiry time.Time
	e = tx.QueryRow(ctx, `SELECT grid,status,version,expires_at FROM platform.catalog_imports WHERE id=$1 FOR UPDATE`, id).Scan(&raw, &state, &v, &expiry)
	if e != nil {
		return nil, dbError(e)
	}
	if v != version || (state != "UPLOADED" && state != "PREVIEW") || !expiry.After(time.Now()) {
		return nil, conflict()
	}
	var grid Grid
	if json.Unmarshal(raw, &grid) != nil {
		return nil, auth.Unavailable()
	}
	lines, e := normalize(grid, m)
	if e != nil {
		return nil, e
	}
	if e = lockCatalog(ctx, tx); e != nil {
		return nil, dbError(e)
	}
	preview := ImportPreview{Total: len(lines), Lines: lines}
	for k := range preview.Lines {
		line := &preview.Lines[k]
		if line.State == "DUPLICATE" {
			preview.Duplicates++
			continue
		}
		if line.State == "ERROR" {
			preview.Errors++
			continue
		}
		in := line.Input
		var old Input
		var category, ps, vs, is, status string
		var reserved int64
		var variants int
		e = tx.QueryRow(ctx, `SELECT p.name,p.description,p.brand,coalesce(c.name,''),p.status,p.catalog_source,v.catalog_source,i.catalog_source,v.variant_name,v.attributes,v.price_amount,v.currency_code,i.quantity_on_hand,i.quantity_reserved,(SELECT count(*) FROM platform.product_variants WHERE product_id=p.id) FROM platform.product_variants v JOIN platform.products p ON p.id=v.product_id JOIN platform.inventory i ON i.variant_id=v.id LEFT JOIN platform.product_categories c ON c.id=p.category_id WHERE v.sku=$1`, in.SKU).Scan(&old.Name, &old.Description, &old.Brand, &category, &status, &ps, &vs, &is, &old.VariantName, &old.Attributes, &old.PriceAmount, &old.CurrencyCode, &old.QuantityOnHand, &reserved, &variants)
		if e == pgx.ErrNoRows {
			line.State = "NEW"
			preview.New++
			continue
		}
		if e != nil {
			return nil, dbError(e)
		}
		line.OldPrice = &old.PriceAmount
		line.OldStock = &old.QuantityOnHand
		productChanged := old.Name != in.Name || old.Description != in.Description || old.Brand != in.Brand || !strings.EqualFold(category, line.Category)
		attrsOld, _ := json.Marshal(old.Attributes)
		attrsNew, _ := json.Marshal(in.Attributes)
		variantChanged := old.PriceAmount != in.PriceAmount || old.CurrencyCode != in.CurrencyCode || string(attrsOld) != string(attrsNew) || old.VariantName != in.VariantName
		stockChanged := old.QuantityOnHand != in.QuantityOnHand
		switch {
		case status == "archived" || in.QuantityOnHand < reserved || status == "active" && in.PriceAmount == 0:
			line.State = "CONFLICT"
			line.Reason = "ARCHIVED_OR_RESERVED_OR_ACTIVE_PRICE"
		case productChanged && (ps != "EXCEL" || variants > 1) || variantChanged && vs != "EXCEL" || stockChanged && is != "EXCEL":
			line.State = "CONFLICT"
			line.Reason = "SOURCE_CONFLICT"
		case !productChanged && !variantChanged && !stockChanged:
			line.State = "UNCHANGED"
		default:
			line.State = "UPDATED"
		}
		switch line.State {
		case "CONFLICT":
			preview.Conflicts++
		case "UNCHANGED":
			preview.Unchanged++
		case "UPDATED":
			preview.Updated++
		}
	}
	f, e := fingerprint(ctx, tx)
	if e != nil {
		return nil, dbError(e)
	}
	p, _ := json.Marshal(preview)
	mapping, _ := json.Marshal(m)
	_, e = tx.Exec(ctx, `UPDATE platform.catalog_imports SET mapping=$2,preview=$3,catalog_fingerprint=$4,status='PREVIEW',version=version+1 WHERE id=$1`, id, mapping, p, f)
	if e != nil {
		return nil, dbError(e)
	}
	if e = audit(ctx, tx, t, id, "IMPORT_PREVIEWED"); e != nil {
		return nil, e
	}
	result, e := importObject(ctx, tx, id)
	return commit(ctx, tx, result, e)
}
func (s Service) ConfirmImport(ctx context.Context, t Tenant, id string, version int64, confirm bool) (json.RawMessage, error) {
	if !organizations.ValidID(id) || version < 1 || !confirm {
		return nil, invalid("Нужны версия preview и явное подтверждение")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	var state, baseline string
	var v int64
	var expiry time.Time
	e = tx.QueryRow(ctx, `SELECT status,version,preview,coalesce(catalog_fingerprint,''),expires_at FROM platform.catalog_imports WHERE id=$1 FOR UPDATE`, id).Scan(&state, &v, &raw, &baseline, &expiry)
	if e != nil {
		return nil, dbError(e)
	}
	if state == "APPLIED" {
		if v != version+1 {
			return nil, conflict()
		}
		return importObject(ctx, tx, id)
	}
	if state != "PREVIEW" || v != version || !expiry.After(time.Now()) {
		return nil, conflict()
	}
	if e = lockCatalog(ctx, tx); e != nil {
		return nil, dbError(e)
	}
	f, e := fingerprint(ctx, tx)
	if e != nil {
		return nil, dbError(e)
	}
	if f != baseline {
		return rejectedImport(ctx, tx, t, id, "IMPORT_STALE", auth.Failure(409, "STALE_PREVIEW", "Каталог изменился; повторите предпросмотр"))
	}
	var preview ImportPreview
	if json.Unmarshal(raw, &preview) != nil {
		return nil, auth.Unavailable()
	}
	if preview.Errors+preview.Conflicts+preview.Duplicates > 0 {
		return rejectedImport(ctx, tx, t, id, "IMPORT_BLOCKED", auth.Failure(409, "IMPORT_BLOCKED", "Исправьте ошибки, дубликаты и конфликты; частичного применения нет"))
	}
	if _, e = tx.Exec(ctx, `SELECT set_config('app.catalog_writer','EXCEL',true)`); e != nil {
		return nil, dbError(e)
	}
	for _, line := range preview.Lines {
		if line.State != "UNCHANGED" {
			if line.State != "NEW" && line.State != "UPDATED" {
				return nil, invalid("Недопустимое состояние строки")
			}
			if e = s.applyLine(ctx, tx, t, line); e != nil {
				return nil, e
			}
		}
		// Confirmed authoritative snapshots verify unchanged and zero stock as well.
		if _, e = tx.Exec(ctx, `UPDATE platform.inventory SET manual_updated_at=now(),version=version+1 WHERE catalog_source='EXCEL' AND variant_id IN(SELECT id FROM platform.product_variants WHERE sku=$1)`, line.Input.SKU); e != nil {
			return nil, dbError(e)
		}
	}

	_, e = tx.Exec(ctx, `UPDATE platform.catalog_imports SET status='APPLIED',version=version+1,grid='[]'::jsonb,applied_at=now(),summary=preview-'lines' WHERE id=$1`, id)
	if e != nil {
		return nil, dbError(e)
	}
	_, e = tx.Exec(ctx, `UPDATE platform.store_profiles SET status=CASE WHEN status IN ('IN_PROGRESS','NOT_STARTED') THEN 'CATALOG_READY' ELSE status END,version=version+1,updated_at=now() WHERE organization_id=$1`, t.OrganizationID)
	if e == nil {
		e = audit(ctx, tx, t, id, "IMPORT_APPLIED")
	}
	if e != nil {
		return nil, dbErrorOrNil(e)
	}
	data, e := importObject(ctx, tx, id)
	return commit(ctx, tx, data, e)
}
func (s Service) applyLine(ctx context.Context, tx pgx.Tx, t Tenant, line ImportLine) error {
	in := line.Input
	var cat *string
	if line.Category != "" {
		var id string
		e := tx.QueryRow(ctx, `SELECT id FROM platform.product_categories WHERE lower(name)=lower($1)`, line.Category).Scan(&id)
		if e == pgx.ErrNoRows {
			id = auth.UUID()
			_, e = tx.Exec(ctx, `INSERT INTO platform.product_categories(id,organization_id,name) VALUES($1,$2,$3)`, id, t.OrganizationID, line.Category)
		}
		if e != nil {
			return dbError(e)
		}
		cat = &id
	}
	attrs, _ := json.Marshal(in.Attributes)
	pid, vid := auth.UUID(), auth.UUID()
	if line.State == "NEW" {
		_, e := tx.Exec(ctx, `INSERT INTO platform.products(id,organization_id,category_id,name,description,brand,catalog_source) VALUES($1,$2,$3,$4,$5,$6,'EXCEL')`, pid, t.OrganizationID, cat, in.Name, in.Description, in.Brand)
		if e != nil {
			return dbError(e)
		}
		_, e = tx.Exec(ctx, `INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,attributes,price_amount,currency_code,catalog_source) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'EXCEL')`, vid, t.OrganizationID, pid, in.SKU, in.VariantName, attrs, in.PriceAmount, in.CurrencyCode)
		if e != nil {
			return dbError(e)
		}
		_, e = tx.Exec(ctx, `INSERT INTO platform.inventory(organization_id,variant_id,catalog_source) VALUES($1,$2,'EXCEL')`, t.OrganizationID, vid)
		if e != nil {
			return dbError(e)
		}
	} else {
		if e := tx.QueryRow(ctx, `SELECT product_id,id FROM platform.product_variants WHERE sku=$1`, in.SKU).Scan(&pid, &vid); e != nil {
			return dbError(e)
		}
		_, e := tx.Exec(ctx, `UPDATE platform.products SET name=$2,description=$3,brand=$4,category_id=$5,version=version+1,updated_at=now() WHERE id=$1 AND (name,description,brand,category_id) IS DISTINCT FROM ($2::text,$3::text,$4::text,$5::uuid)`, pid, in.Name, in.Description, in.Brand, cat)
		if e != nil {
			return dbError(e)
		}
		_, e = tx.Exec(ctx, `UPDATE platform.product_variants SET variant_name=$2,attributes=$3,price_amount=$4,currency_code=$5,version=version+1,updated_at=now() WHERE id=$1 AND (variant_name,attributes,price_amount,currency_code) IS DISTINCT FROM ($2::text,$3::jsonb,$4::bigint,$5::text)`, vid, in.VariantName, attrs, in.PriceAmount, in.CurrencyCode)
		if e != nil {
			return dbError(e)
		}
	}
	var before, reserved, version int64
	if e := tx.QueryRow(ctx, `SELECT quantity_on_hand,quantity_reserved,version FROM platform.inventory WHERE variant_id=$1 FOR UPDATE`, vid).Scan(&before, &reserved, &version); e != nil {
		return dbError(e)
	}
	if before != in.QuantityOnHand {
		_, e := tx.Exec(ctx, `UPDATE platform.inventory SET quantity_on_hand=$2,version=version+1,updated_at=now(),manual_updated_at=now() WHERE variant_id=$1`, vid, in.QuantityOnHand)
		if e != nil {
			return dbError(e)
		}
		_, e = tx.Exec(ctx, `INSERT INTO platform.inventory_movements(id,organization_id,variant_id,actor_id,reason,quantity_before,quantity_after,delta,inventory_version,reserved_before,reserved_after) VALUES($1,$2,$3,platform.current_user_id(),$4,$5,$6,$7,$8,$9,$9)`, auth.UUID(), t.OrganizationID, vid, fmt.Sprintf("Excel import row %d", line.Row), before, in.QuantityOnHand, in.QuantityOnHand-before, version+1, reserved)
		if e != nil {
			return dbError(e)
		}
	}
	return nil
}
func (s Service) CancelImport(ctx context.Context, t Tenant, id string, version int64) (json.RawMessage, error) {
	if !organizations.ValidID(id) || version < 1 {
		return nil, invalid("Нужен импорт и версия")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var state string
	var v int64
	e = tx.QueryRow(ctx, `SELECT status,version FROM platform.catalog_imports WHERE id=$1 FOR UPDATE`, id).Scan(&state, &v)
	if e != nil {
		return nil, dbError(e)
	}
	if state == "CANCELLED" && v == version+1 {
		return importObject(ctx, tx, id)
	}
	if v != version || state == "APPLIED" {
		return nil, conflict()
	}
	_, e = tx.Exec(ctx, `UPDATE platform.catalog_imports SET status='CANCELLED',version=version+1,grid='[]'::jsonb WHERE id=$1`, id)
	if e != nil {
		return nil, dbError(e)
	}
	if e = audit(ctx, tx, t, id, "IMPORT_CANCELLED"); e != nil {
		return nil, e
	}
	data, e := importObject(ctx, tx, id)
	return commit(ctx, tx, data, e)
}

func rejectedImport(ctx context.Context, tx pgx.Tx, t Tenant, id, kind string, reason error) (json.RawMessage, error) {
	if e := audit(ctx, tx, t, id, kind); e != nil {
		return nil, e
	}
	if e := tx.Commit(ctx); e != nil {
		return nil, dbError(e)
	}
	return nil, reason
}
