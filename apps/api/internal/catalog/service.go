// Package catalog owns tenant-scoped catalogue and single-warehouse inventory.
package catalog

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Service struct {
	Pool         *pgxpool.Pool
	CleanupProof []byte
}

// Tenant is supplied by the authenticated backend, never deserialized from tool input.
type Tenant struct {
	AgentTaskID, AgentLease string // Internal read lease, never accepted from API payload.
	Session                 auth.Session
	OrganizationID          string
	ChannelType             string // Trusted adapter identifier, never taken from HTTP/tool input.
	ChannelCapability       string // Ephemeral inbox lease; never populated from HTTP or LLM input.
}
type Input struct {
	Name            string            `json:"name"`
	CategoryID      *string           `json:"category_id"`
	Description     string            `json:"description"`
	Brand           string            `json:"brand"`
	Status          string            `json:"status"`
	SKU             string            `json:"sku"`
	VariantName     string            `json:"variant_name"`
	Attributes      map[string]string `json:"attributes"`
	PriceAmount     int64             `json:"price_amount"`
	CurrencyCode    string            `json:"currency_code"`
	ExpectedVersion int64             `json:"expected_version"`
	VariantID       string            `json:"variant_id"`
	QuantityOnHand  int64             `json:"quantity_on_hand"`
	Reason          string            `json:"reason"`
}
type Filters struct {
	Query, CategoryID, Status, Stock, Sort string
	Page, PageSize                         int
}

func invalid(message string) error { return auth.Failure(422, "VALIDATION_ERROR", message) }
func missing() error               { return auth.Failure(404, "NOT_FOUND", "Ресурс не найден") }
func conflict() error {
	return auth.Failure(409, "VERSION_CONFLICT", "Данные изменились. Обновите страницу")
}
func dbError(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return missing()
	}
	var p *pgconn.PgError
	if errors.As(e, &p) {
		switch p.Code {
		case "P0010":
			return auth.Failure(409, "SOURCE_CONFLICT", "Данные управляются другим источником; используйте исходный источник")
		case "23505":
			return auth.Failure(409, "DUPLICATE_VALUE", "SKU или название категории уже используются")
		case "23503", "23514", "22001", "22003":
			return invalid("Некорректная ссылка или значение")
		case "42501":
			return auth.Failure(403, "FORBIDDEN", "Операция запрещена")
		}
	}
	return auth.Unavailable()
}
func textValid(s string, min, max int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(s))
	return n >= min && n <= max && !strings.ContainsRune(s, 0) && utf8.ValidString(s)
}

var skuRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,63}$`)

func (in *Input) validateVariant() error {
	in.SKU = strings.ToUpper(strings.TrimSpace(in.SKU))
	in.VariantName = strings.TrimSpace(in.VariantName)
	if in.VariantName == "" {
		in.VariantName = "Основной"
	}
	if in.CurrencyCode == "" {
		in.CurrencyCode = "RUB"
	}
	if in.Attributes == nil {
		in.Attributes = map[string]string{}
	}
	if !skuRE.MatchString(in.SKU) || !textValid(in.VariantName, 1, 120) || in.PriceAmount < 0 || in.PriceAmount > 9000000000000 || in.CurrencyCode != "RUB" || len(in.Attributes) > 32 {
		return invalid("Проверьте SKU, название варианта, цену и валюту RUB")
	}
	data, e := json.Marshal(in.Attributes)
	if e != nil || len(data) > 4096 {
		return invalid("Характеристики: максимум 4096 байт")
	}
	for k, v := range in.Attributes {
		if !textValid(k, 1, 64) || !textValid(v, 0, 256) {
			return invalid("Характеристики: ключ 1–64, значение до 256 символов")
		}
	}
	return nil
}
func (in *Input) validateProduct() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Brand = strings.TrimSpace(in.Brand)
	if !textValid(in.Name, 1, 200) || !textValid(in.Description, 0, 10000) || !textValid(in.Brand, 0, 100) {
		return invalid("Название 1–200, описание до 10000, бренд до 100 символов")
	}
	if in.CategoryID != nil && !organizations.ValidID(*in.CategoryID) {
		return invalid("Некорректная категория")
	}
	return nil
}
func (s Service) begin(ctx context.Context, t Tenant, write bool) (pgx.Tx, error) {
	return s.beginAccess(ctx, t, write, false)
}

// Owner control reads may run together, while the shared tenant lock still
// fences every existing exclusive catalog writer for the entire snapshot.
func (s Service) beginOwnerRead(ctx context.Context, t Tenant) (pgx.Tx, error) {
	return s.beginAccess(ctx, t, true, true)
}

func (s Service) beginAccess(ctx context.Context, t Tenant, write, shared bool) (pgx.Tx, error) {
	if !organizations.ValidID(t.OrganizationID) {
		return nil, missing()
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, auth.Unavailable()
	}
	_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(t.Session.Hash), t.OrganizationID)
	if e != nil {
		_ = tx.Rollback(ctx)
		return nil, dbError(e)
	}
	if t.AgentTaskID != "" || t.AgentLease != "" {
		var allowed bool
		if write || t.ChannelCapability != "" || len(t.Session.Hash) != 0 || !organizations.ValidID(t.AgentTaskID) || !organizations.ValidID(t.AgentLease) {
			_ = tx.Rollback(ctx)
			return nil, missing()
		}
		_, e = tx.Exec(ctx, "SELECT set_config('app.agent_task',$1,true),set_config('app.agent_lease',$2,true)", t.AgentTaskID, t.AgentLease)
		if e == nil {
			e = tx.QueryRow(ctx, "SELECT platform.agent_catalog_scope($1)", t.OrganizationID).Scan(&allowed)
		}
		if e != nil || !allowed {
			_ = tx.Rollback(ctx)
			return nil, missing()
		}
		return tx, nil
	}
	if t.ChannelCapability != "" {
		if write {
			_ = tx.Rollback(ctx)
			return nil, auth.Failure(403, "FORBIDDEN", "Channel principal is read-only")
		}
		setting, scope := "app.telegram_cap", "platform.telegram_scope"
		if t.ChannelType == "whatsapp" || t.ChannelType == "instagram" {
			setting, scope = "app.channel_cap", "platform.channel_scope"
		}
		_, e = tx.Exec(ctx, `SELECT set_config($1,$2,true),set_config('app.channel_type',$3,true)`, setting, t.ChannelCapability, t.ChannelType)
		var allowed bool
		if e == nil {
			e = tx.QueryRow(ctx, `SELECT `+scope+`($1,NULL)`, t.OrganizationID).Scan(&allowed)
		}
		if e != nil || !allowed {
			_ = tx.Rollback(ctx)
			return nil, missing()
		}
		return tx, nil
	}
	var role string
	e = tx.QueryRow(ctx, `SELECT m.role FROM platform.organizations o JOIN platform.organization_members m ON m.organization_id=o.id WHERE o.id=$1`, t.OrganizationID).Scan(&role)
	if e != nil {
		_ = tx.Rollback(ctx)
		return nil, dbError(e)
	}
	if write && role != "owner" {
		_ = tx.Rollback(ctx)
		return nil, auth.Failure(403, "FORBIDDEN", "Только владелец может изменять каталог")
	}
	if write {
		lockSQL := `SELECT pg_advisory_xact_lock(hashtextextended($1,10130))`
		if shared {
			lockSQL = `SELECT pg_advisory_xact_lock_shared(hashtextextended($1,10130))`
		}
		if _, e = tx.Exec(ctx, lockSQL, t.OrganizationID); e != nil {
			_ = tx.Rollback(ctx)
			return nil, dbError(e)
		}
	}
	return tx, nil
}
func object(ctx context.Context, tx pgx.Tx, sql string, args ...any) (json.RawMessage, error) {
	var data []byte
	e := tx.QueryRow(ctx, sql, args...).Scan(&data)
	if e != nil {
		return nil, dbError(e)
	}
	return json.RawMessage(data), nil
}
func items(ctx context.Context, tx pgx.Tx, sql string, args ...any) (json.RawMessage, error) {
	return object(ctx, tx, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(to_jsonb(x)),'[]'::jsonb)) FROM (`+sql+`) x`, args...)
}
func commit(ctx context.Context, tx pgx.Tx, data json.RawMessage, e error) (json.RawMessage, error) {
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, dbError(e)
	}
	return data, nil
}
func (s Service) Categories(ctx context.Context, t Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t, false)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	return items(ctx, tx, `SELECT * FROM platform.product_categories ORDER BY lower(name),id`)
}
func (s Service) Category(ctx context.Context, t Tenant, id string, in Input) (json.RawMessage, error) {
	if !textValid(in.Name, 1, 120) {
		return nil, invalid("Название категории: 1–120 символов")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var data json.RawMessage
	if id == "" {
		data, e = object(ctx, tx, `INSERT INTO platform.product_categories(id,organization_id,name) VALUES($1,$2,$3) RETURNING to_jsonb(product_categories.*)`, auth.UUID(), t.OrganizationID, strings.TrimSpace(in.Name))
	} else {
		if !organizations.ValidID(id) || in.ExpectedVersion < 1 {
			return nil, invalid("Некорректный ID или версия")
		}
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.product_categories WHERE id=$1)`, id).Scan(&exists); e != nil {
			return nil, dbError(e)
		}
		if !exists {
			return nil, missing()
		}
		data, e = object(ctx, tx, `UPDATE platform.product_categories SET name=$2,version=version+1,updated_at=now() WHERE id=$1 AND version=$3 RETURNING to_jsonb(product_categories.*)`, id, strings.TrimSpace(in.Name), in.ExpectedVersion)
		if e != nil {
			var ae *auth.Error
			if errors.As(e, &ae) && ae.Status == 404 {
				e = conflict()
			}
		}
	}
	return commit(ctx, tx, data, e)
}
func (s Service) CreateProduct(ctx context.Context, t Tenant, in Input) (json.RawMessage, error) {
	if e := in.validateProduct(); e != nil {
		return nil, e
	}
	if e := in.validateVariant(); e != nil {
		return nil, e
	}
	if in.Status != "" && in.Status != "draft" {
		return nil, invalid("Новый товар создаётся в draft")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	id := auth.UUID()
	_, e = tx.Exec(ctx, `INSERT INTO platform.products(id,organization_id,category_id,name,description,brand) VALUES($1,$2,$3,$4,$5,$6)`, id, t.OrganizationID, in.CategoryID, in.Name, in.Description, in.Brand)
	if e != nil {
		return nil, dbError(e)
	}
	if _, e = s.variant(ctx, tx, t, id, "", in); e != nil {
		return nil, e
	}
	data, e := product(ctx, tx, id, false)
	return commit(ctx, tx, data, e)
}
func product(ctx context.Context, tx pgx.Tx, id string, customer bool) (json.RawMessage, error) {
	return object(ctx, tx, `SELECT to_jsonb(p)||jsonb_build_object('variants',coalesce((SELECT jsonb_agg(to_jsonb(v)||jsonb_build_object('inventory',(CASE WHEN $2 THEN (to_jsonb(i)-'quantity_on_hand'-'quantity_reserved') ELSE to_jsonb(i) END)||jsonb_build_object('freshness',platform.inventory_freshness(i.organization_id,i.manual_updated_at),'available',CASE WHEN NOT $2 OR platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH' THEN i.quantity_on_hand-i.quantity_reserved ELSE NULL END)) ORDER BY v.created_at,v.id) FROM platform.product_variants v JOIN platform.inventory i ON i.organization_id=v.organization_id AND i.variant_id=v.id WHERE v.product_id=p.id),'[]'::jsonb)) FROM platform.products p WHERE p.id=$1 AND (NOT $2 OR p.status='active')`, id, customer)
}
func (s Service) Product(ctx context.Context, t Tenant, id string, customer bool) (json.RawMessage, error) {
	if !organizations.ValidID(id) {
		return nil, missing()
	}
	tx, e := s.begin(ctx, t, false)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	return product(ctx, tx, id, customer)
}
func validateFilters(f *Filters) error {
	if f.Page == 0 {
		f.Page = 1
	}
	if f.PageSize == 0 {
		f.PageSize = 20
	}
	if f.Sort == "" {
		f.Sort = "newest"
	}
	if f.Page < 1 || f.Page > 10000 || f.PageSize < 1 || f.PageSize > 100 || !textValid(f.Query, 0, 200) || f.CategoryID != "" && !organizations.ValidID(f.CategoryID) {
		return invalid("Некорректная пагинация или поиск")
	}
	if f.Status != "" && f.Status != "draft" && f.Status != "active" && f.Status != "archived" {
		return invalid("Некорректный статус")
	}
	if f.Stock != "" && f.Stock != "in_stock" && f.Stock != "out_of_stock" {
		return invalid("Некорректный фильтр наличия")
	}
	if f.Sort != "newest" && f.Sort != "name" && f.Sort != "price_asc" && f.Sort != "price_desc" {
		return invalid("Некорректная сортировка")
	}
	return nil
}
func (s Service) Products(ctx context.Context, t Tenant, f Filters, customer bool) (json.RawMessage, error) {
	if customer {
		f.Status = "active"
	}
	if e := validateFilters(&f); e != nil {
		return nil, e
	}
	tx, e := s.begin(ctx, t, false)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	// Keep LLVM compilation of the RLS-heavy projection outside the budget of
	// this interactive query. The pooled connection keeps its original setting.
	if _, e = tx.Exec(ctx, "SET LOCAL jit=off"); e != nil {
		return nil, dbError(e)
	}
	order := map[string]string{"newest": "created_at DESC,id", "name": "lower(name),id", "price_asc": "price_amount,id", "price_desc": "price_amount DESC,id"}[f.Sort]
	// Literal substring matching escapes LIKE metacharacters. All filters remain parameters.
	q := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(f.Query)
	// Explicit UUID tenant keys bound correlated scans before RLS costing; RLS still enforces access.
	sql := `WITH matched AS (SELECT p.*, (SELECT min(v.price_amount) FROM platform.product_variants v WHERE v.organization_id=p.organization_id AND v.product_id=p.id) price_amount, (SELECT coalesce(sum(i.quantity_on_hand-i.quantity_reserved),0) FROM platform.product_variants v JOIN platform.inventory i ON i.organization_id=v.organization_id AND i.variant_id=v.id WHERE v.organization_id=p.organization_id AND v.product_id=p.id) available FROM platform.products p WHERE p.organization_id=current_setting('app.organization_id')::uuid AND ($1='' OR p.name ILIKE '%'||$1||'%' OR p.brand ILIKE '%'||$1||'%' OR EXISTS(SELECT 1 FROM platform.product_variants v WHERE v.organization_id=p.organization_id AND v.product_id=p.id AND v.sku ILIKE '%'||$1||'%')) AND ($2='' OR p.category_id::text=$2) AND ($3='' OR p.status=$3)), filtered AS (SELECT * FROM matched WHERE ($4='' OR ($4='in_stock' AND available>0) OR ($4='out_of_stock' AND available=0))) SELECT jsonb_build_object('items',coalesce((SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT * FROM filtered ORDER BY ` + order + ` LIMIT $5 OFFSET $6) x),'[]'::jsonb),'total',(SELECT count(*) FROM filtered),'page',$7::integer,'page_size',$5::integer)`
	if customer {
		sql = strings.Replace(sql, "coalesce(sum(i.quantity_on_hand-i.quantity_reserved),0)", "CASE WHEN bool_and(platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH') THEN sum(i.quantity_on_hand-i.quantity_reserved) ELSE NULL END", 1)
	}
	return object(ctx, tx, sql, q, f.CategoryID, f.Status, f.Stock, f.PageSize, (f.Page-1)*f.PageSize, f.Page)
}
func (s Service) UpdateProduct(ctx context.Context, t Tenant, id string, in Input, action string) (json.RawMessage, error) {
	if !organizations.ValidID(id) || in.ExpectedVersion < 1 {
		return nil, invalid("Некорректный ID или версия")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var status string
	var version int64
	e = tx.QueryRow(ctx, `SELECT status,version FROM platform.products WHERE id=$1 FOR UPDATE`, id).Scan(&status, &version)
	if e != nil {
		return nil, dbError(e)
	}
	if version != in.ExpectedVersion {
		return nil, conflict()
	}
	if status == "archived" {
		return nil, auth.Failure(409, "INVALID_STATE", "Архивный товар нельзя изменить")
	}
	if action != "" {
		in.Status = map[string]string{"publish": "active", "archive": "archived"}[action]
	} else {
		if e = in.validateProduct(); e != nil {
			return nil, e
		}
		if in.Status == "" {
			in.Status = status
		}
		if in.Status != "draft" && in.Status != "active" {
			return nil, invalid("Для архивирования используйте archive")
		}
	}
	if in.Status == "active" {
		var valid bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.product_variants WHERE product_id=$1) AND NOT EXISTS(SELECT 1 FROM platform.product_variants v LEFT JOIN platform.inventory i ON i.organization_id=v.organization_id AND i.variant_id=v.id WHERE v.product_id=$1 AND (v.price_amount<=0 OR i.variant_id IS NULL))`, id).Scan(&valid)
		if e != nil {
			return nil, dbError(e)
		}
		if !valid {
			return nil, invalid("Публикация требует вариантов с положительной ценой и остатком")
		}
	}
	if action != "" {
		_, e = tx.Exec(ctx, `UPDATE platform.products SET status=$2,version=version+1,updated_at=now() WHERE id=$1`, id, in.Status)
	} else {
		_, e = tx.Exec(ctx, `UPDATE platform.products SET name=$2,category_id=$3,description=$4,brand=$5,status=$6,version=version+1,updated_at=now() WHERE id=$1`, id, in.Name, in.CategoryID, in.Description, in.Brand, in.Status)
	}
	if e != nil {
		return nil, dbError(e)
	}
	data, e := product(ctx, tx, id, false)
	return commit(ctx, tx, data, e)
}
func (s Service) variant(ctx context.Context, tx pgx.Tx, t Tenant, productID, id string, in Input) (json.RawMessage, error) {
	var data json.RawMessage
	var e error
	attrs, _ := json.Marshal(in.Attributes)
	if id == "" {
		id = auth.UUID()
		data, e = object(ctx, tx, `INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,attributes,price_amount,currency_code) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING to_jsonb(product_variants.*)`, id, t.OrganizationID, productID, in.SKU, in.VariantName, attrs, in.PriceAmount, in.CurrencyCode)
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO platform.inventory(organization_id,variant_id) VALUES($1,$2)`, t.OrganizationID, id)
			if e != nil {
				e = dbError(e)
			}
		}
	} else {
		data, e = object(ctx, tx, `UPDATE platform.product_variants SET sku=$2,variant_name=$3,attributes=$4,price_amount=$5,currency_code=$6,version=version+1,updated_at=now() WHERE id=$1 AND version=$7 RETURNING to_jsonb(product_variants.*)`, id, in.SKU, in.VariantName, attrs, in.PriceAmount, in.CurrencyCode, in.ExpectedVersion)
		var ae *auth.Error
		if errors.As(e, &ae) && ae.Status == 404 {
			e = conflict()
		}
	}
	return data, e
}
func (s Service) Variant(ctx context.Context, t Tenant, productID, id string, in Input) (json.RawMessage, error) {
	if e := in.validateVariant(); e != nil {
		return nil, e
	}
	if id != "" && (!organizations.ValidID(id) || in.ExpectedVersion < 1) {
		return nil, invalid("Некорректный вариант или версия")
	}
	if id == "" && !organizations.ValidID(productID) {
		return nil, missing()
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if id != "" {
		e = tx.QueryRow(ctx, `SELECT product_id FROM platform.product_variants WHERE id=$1`, id).Scan(&productID)
		if e != nil {
			return nil, dbError(e)
		}
	}
	var status string
	e = tx.QueryRow(ctx, `SELECT status FROM platform.products WHERE id=$1 FOR UPDATE`, productID).Scan(&status)
	if e != nil {
		return nil, dbError(e)
	}
	if status == "archived" {
		return nil, auth.Failure(409, "INVALID_STATE", "Архивный товар нельзя изменить")
	}
	if status == "active" && in.PriceAmount <= 0 {
		return nil, invalid("Активный товар требует положительной цены")
	}
	data, e := s.variant(ctx, tx, t, productID, id, in)
	return commit(ctx, tx, data, e)
}
func (s Service) Variants(ctx context.Context, t Tenant, id string) (json.RawMessage, error) {
	if !organizations.ValidID(id) {
		return nil, missing()
	}
	tx, e := s.begin(ctx, t, false)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = product(ctx, tx, id, false); e != nil {
		return nil, e
	}
	return items(ctx, tx, `SELECT * FROM platform.product_variants WHERE product_id=$1 ORDER BY created_at,id`, id)
}
func (s Service) Inventory(ctx context.Context, t Tenant, f Filters, movements bool) (json.RawMessage, error) {
	if e := validateFilters(&f); e != nil {
		return nil, e
	}
	tx, e := s.begin(ctx, t, false)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if movements {
		return items(ctx, tx, `SELECT m.*,v.sku,v.variant_name,p.name product_name FROM platform.inventory_movements m JOIN platform.product_variants v ON v.id=m.variant_id AND v.organization_id=m.organization_id JOIN platform.products p ON p.id=v.product_id AND p.organization_id=v.organization_id ORDER BY m.created_at DESC,m.id LIMIT $1 OFFSET $2`, f.PageSize, (f.Page-1)*f.PageSize)
	}
	return items(ctx, tx, `SELECT i.*,i.quantity_on_hand-i.quantity_reserved available,v.sku,v.variant_name,p.id product_id,p.name product_name,p.status FROM platform.inventory i JOIN platform.product_variants v ON v.id=i.variant_id AND v.organization_id=i.organization_id JOIN platform.products p ON p.id=v.product_id AND p.organization_id=v.organization_id ORDER BY lower(p.name),v.sku LIMIT $1 OFFSET $2`, f.PageSize, (f.Page-1)*f.PageSize)
}
func (s Service) Adjust(ctx context.Context, t Tenant, in Input) (json.RawMessage, error) {
	if !organizations.ValidID(in.VariantID) || in.ExpectedVersion < 1 || in.QuantityOnHand < 0 || in.QuantityOnHand > 1000000000 || !textValid(in.Reason, 1, 500) {
		return nil, invalid("Проверьте вариант, версию, количество и причину")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var before, reserved int64
	var status string
	e = tx.QueryRow(ctx, `SELECT p.status FROM platform.product_variants v JOIN platform.products p ON p.id=v.product_id AND p.organization_id=v.organization_id WHERE v.id=$1 FOR UPDATE OF p`, in.VariantID).Scan(&status)
	if e != nil {
		return nil, dbError(e)
	}
	if status == "archived" {
		return nil, auth.Failure(409, "INVALID_STATE", "Архивный товар нельзя изменить")
	}
	var version int64
	e = tx.QueryRow(ctx, `SELECT quantity_on_hand,quantity_reserved,version FROM platform.inventory WHERE variant_id=$1 FOR UPDATE`, in.VariantID).Scan(&before, &reserved, &version)
	if e != nil {
		return nil, dbError(e)
	}
	if version != in.ExpectedVersion {
		return nil, conflict()
	}
	if in.QuantityOnHand < reserved {
		return nil, invalid("Остаток не может быть меньше резерва")
	}
	data, e := object(ctx, tx, `UPDATE platform.inventory SET quantity_on_hand=$2,version=version+1,updated_at=now(),manual_updated_at=now() WHERE variant_id=$1 AND version=$3 RETURNING to_jsonb(inventory.*)||jsonb_build_object('available',quantity_on_hand-quantity_reserved)`, in.VariantID, in.QuantityOnHand, in.ExpectedVersion)
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform.inventory_movements(id,organization_id,variant_id,actor_id,reason,quantity_before,quantity_after,delta,inventory_version,reserved_before,reserved_after) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)`, auth.UUID(), t.OrganizationID, in.VariantID, t.Session.User.ID, strings.TrimSpace(in.Reason), before, in.QuantityOnHand, in.QuantityOnHand-before, version+1, reserved)
	if e != nil {
		return nil, dbError(e)
	}
	return commit(ctx, tx, data, nil)
}

// Customer-facing reads are internal only and always exclude draft/archived products.
type CustomerCatalog struct{ service Service }

func (s Service) Customer() CustomerCatalog { return CustomerCatalog{service: s} }
func (s CustomerCatalog) SearchProducts(ctx context.Context, t Tenant, f Filters) (json.RawMessage, error) {
	return s.service.Products(ctx, t, f, true)
}
func (s CustomerCatalog) GetProductDetails(ctx context.Context, t Tenant, id string) (json.RawMessage, error) {
	return s.service.Product(ctx, t, id, true)
}
func (s CustomerCatalog) CheckInventory(ctx context.Context, t Tenant, id string) (json.RawMessage, error) {
	return s.service.customerVariant(ctx, t, id, false)
}
func (s CustomerCatalog) GetProductPrice(ctx context.Context, t Tenant, id string) (json.RawMessage, error) {
	return s.service.customerVariant(ctx, t, id, true)
}
func (s Service) customerVariant(ctx context.Context, t Tenant, id string, price bool) (json.RawMessage, error) {
	if !organizations.ValidID(id) {
		return nil, missing()
	}
	tx, e := s.begin(ctx, t, false)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	fields := `(to_jsonb(i)-'quantity_on_hand'-'quantity_reserved')||jsonb_build_object('freshness',platform.inventory_freshness(i.organization_id,i.manual_updated_at),'available',CASE WHEN platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH' THEN i.quantity_on_hand-i.quantity_reserved ELSE NULL END)`
	if price {
		fields = `jsonb_build_object('variant_id',v.id,'price_amount',v.price_amount,'currency_code',v.currency_code,'version',v.version,'updated_at',v.updated_at)`
	}
	return object(ctx, tx, `SELECT `+fields+` FROM platform.product_variants v JOIN platform.products p ON p.id=v.product_id AND p.organization_id=v.organization_id JOIN platform.inventory i ON i.variant_id=v.id AND i.organization_id=v.organization_id WHERE v.id=$1 AND p.status='active'`, id)
}

// ScopedProductForVariant resolves a variant using the same customer catalog
// policy as details reads. In the worker this transaction is bound to one variant.
func (s CustomerCatalog) ScopedProductForVariant(ctx context.Context, t Tenant, variantID string) (string, error) {
	if !organizations.ValidID(variantID) {
		return "", missing()
	}
	tx, err := s.service.begin(ctx, t, false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var productID string
	err = tx.QueryRow(ctx, `SELECT p.id FROM platform.product_variants v JOIN platform.products p
 ON p.organization_id=v.organization_id AND p.id=v.product_id
 WHERE v.id=$1 AND p.status='active'`, variantID).Scan(&productID)
	if err != nil {
		return "", dbError(err)
	}
	return productID, nil
}
