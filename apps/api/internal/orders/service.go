package orders

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"sort"
	"strings"
	"time"
)

type Service struct {
	Pool           *pgxpool.Pool
	StockFreshness time.Duration
}
type CartItem struct {
	VariantID string `json:"variant_id"`
	Quantity  int    `json:"quantity"`
}
type Snapshot struct {
	VariantID       string     `json:"variant_id"`
	ProductID       string     `json:"product_id"`
	Quantity        int        `json:"quantity"`
	Name            string     `json:"name"`
	SKU             string     `json:"sku"`
	UnitPrice       int64      `json:"unit_price_amount"`
	Currency        string     `json:"currency_code"`
	VariantVersion  int64      `json:"variant_version"`
	Available       int        `json:"available"`
	ManualUpdatedAt *time.Time `json:"manual_updated_at"`
	Fresh           bool       `json:"fresh"`
}
type Quote struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	CustomerID     string     `json:"customer_id"`
	Items          []Snapshot `json:"items"`
	ExpiresAt      time.Time  `json:"expires_at"`
	Total          int64      `json:"total_amount"`
	Currency       string     `json:"currency_code"`
	Reserved       bool       `json:"reserved"`
}
type Transition struct {
	ExpectedVersion int64  `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
	Reason          string `json:"reason"`
}

func failure(code, msg string) error { return tenancy.Failure(409, code, msg) }
func validCart(items []CartItem) error {
	if len(items) < 1 || len(items) > 10 {
		return tenancy.Failure(422, "VALIDATION_ERROR", "Заявка: 1–10 позиций")
	}
	seen := map[string]bool{}
	for _, i := range items {
		if !organizations.ValidID(i.VariantID) || i.Quantity < 1 || i.Quantity > 10 || seen[i.VariantID] {
			return tenancy.Failure(422, "VALIDATION_ERROR", "Варианты уникальны, количество 1–10")
		}
		seen[i.VariantID] = true
	}
	return nil
}
func (s Service) fresh(updated *time.Time) bool {
	threshold := s.StockFreshness
	if threshold == 0 {
		threshold = 24 * time.Hour
	}
	return updated != nil && time.Since(*updated) <= threshold
}
func readJSON(ctx context.Context, tx pgx.Tx, sql string, args ...any) (json.RawMessage, error) {
	var raw []byte
	if e := tx.QueryRow(ctx, sql, args...).Scan(&raw); e != nil {
		return nil, tenancy.Error(e)
	}
	return json.RawMessage(raw), nil
}
func detail(ctx context.Context, tx pgx.Tx, id string) (json.RawMessage, error) {
	return readJSON(ctx, tx, `SELECT (CASE WHEN o.request_kind='product_order' THEN to_jsonb(o)-'request_kind'-'request_details'-'listing_id' ELSE to_jsonb(o)||jsonb_build_object('reservation',(SELECT jsonb_build_object('id',h.id,'status',h.status,'expires_at',h.expires_at,'created_at',h.created_at,'booking_id',b.id) FROM platform.hybrid_reservations h JOIN platform.service_bookings b ON b.organization_id=h.organization_id AND b.request_id=h.order_id WHERE h.organization_id=o.organization_id AND h.order_id=o.id)) END)||jsonb_build_object('items',(SELECT coalesce(jsonb_agg(to_jsonb(i) ORDER BY i.variant_id),'[]'::jsonb) FROM platform.order_items i WHERE i.order_id=o.id),'events',(SELECT coalesce(jsonb_agg(to_jsonb(e)-'request_hash'-'result' ORDER BY e.created_at,e.id),'[]'::jsonb) FROM platform.order_events e WHERE e.order_id=o.id)) FROM platform.orders o WHERE o.id=$1`, id)
}
func commit(ctx context.Context, tx pgx.Tx) error {
	if e := tx.Commit(ctx); e != nil {
		return tenancy.Error(e)
	}
	return nil
}

// lockCatalog uses the same product -> inventory lock order as manual adjustments.
func (s Service) lockCatalog(ctx context.Context, tx pgx.Tx, cart []CartItem, active bool) ([]Snapshot, error) {
	if e := validCart(cart); e != nil {
		return nil, e
	}
	quantities := map[string]int{}
	ids := []string{}
	for _, i := range cart {
		ids = append(ids, i.VariantID)
		quantities[i.VariantID] = i.Quantity
	}
	sort.Strings(ids)
	rows, e := tx.Query(ctx, `SELECT DISTINCT product_id::text FROM platform.product_variants WHERE id=ANY($1::uuid[]) ORDER BY product_id::text`, ids)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	productIDs := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, tenancy.Error(e)
		}
		productIDs = append(productIDs, id)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return nil, tenancy.Error(e)
	}
	if _, e = tx.Exec(ctx, `SELECT platform.lock_order_products($1::uuid[])`, productIDs); e != nil {
		return nil, tenancy.Error(e)
	}
	if _, e = tx.Exec(ctx, `SELECT platform.lock_order_inventory($1::uuid[])`, ids); e != nil {
		return nil, tenancy.Error(e)
	}
	rows, e = tx.Query(ctx, `SELECT v.id,v.product_id,p.name,v.sku,v.price_amount,v.currency_code,v.version,i.quantity_on_hand-i.quantity_reserved,i.manual_updated_at,p.status,platform.inventory_freshness(i.organization_id,i.manual_updated_at) FROM platform.product_variants v JOIN platform.products p ON p.id=v.product_id AND p.organization_id=v.organization_id JOIN platform.inventory i ON i.variant_id=v.id AND i.organization_id=v.organization_id WHERE v.id=ANY($1::uuid[]) ORDER BY v.id `, ids)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	defer rows.Close()
	result := []Snapshot{}
	for rows.Next() {
		var item Snapshot
		var status, freshness string
		e = rows.Scan(&item.VariantID, &item.ProductID, &item.Name, &item.SKU, &item.UnitPrice, &item.Currency, &item.VariantVersion, &item.Available, &item.ManualUpdatedAt, &status, &freshness)
		if e != nil {
			return nil, tenancy.Error(e)
		}
		if active && (status != "active" || item.UnitPrice <= 0) {
			return nil, failure("VARIANT_UNAVAILABLE", "Товар больше не доступен для заявки")
		}
		item.Quantity = quantities[item.VariantID]
		item.Fresh = freshness == "FRESH" && s.fresh(item.ManualUpdatedAt)
		result = append(result, item)
	}
	if rows.Err() != nil {
		return nil, tenancy.Error(rows.Err())
	}
	if len(result) != len(cart) {
		return nil, tenancy.Failure(404, "NOT_FOUND", "Вариант не найден")
	}
	return result, nil
}
func conversation(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	if !organizations.ValidID(id) {
		return "", tenancy.Failure(404, "NOT_FOUND", "Диалог не найден")
	}
	var customer, status string
	var expiry time.Time
	e := tx.QueryRow(ctx, `SELECT customer_id,status,expires_at FROM platform.conversations WHERE id=$1 FOR UPDATE`, id).Scan(&customer, &status, &expiry)
	if e != nil {
		return "", tenancy.Error(e)
	}
	if status != "open" || !expiry.After(time.Now()) {
		return "", failure("CONVERSATION_CLOSED", "Диалог закрыт или срок хранения истёк")
	}
	return customer, nil
}
func (s Service) PrepareQuote(ctx context.Context, t catalog.Tenant, conversationID string, cart []CartItem) (Quote, error) {
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return Quote{}, e
	}
	defer tx.Rollback(ctx)
	if e = channelConsent(ctx, tx, t, conversationID); e != nil {
		return Quote{}, e
	}
	customer, e := conversation(ctx, tx, conversationID)
	if e != nil {
		return Quote{}, e
	}
	snapshots, e := s.lockCatalog(ctx, tx, cart, true)
	if e != nil {
		return Quote{}, e
	}
	q := Quote{ID: auth.UUID(), ConversationID: conversationID, CustomerID: customer, Items: snapshots, ExpiresAt: time.Now().Add(5 * time.Minute), Currency: "RUB"}
	for _, i := range snapshots {
		if i.Available < i.Quantity {
			return Quote{}, failure("INSUFFICIENT_STOCK", "Доступное количество уменьшилось. Обратитесь в магазин")
		}
		q.Total += i.UnitPrice * int64(i.Quantity)
	}
	raw, _ := json.Marshal(snapshots)
	_, e = tx.Exec(ctx, `INSERT INTO platform.order_quotes(id,organization_id,conversation_id,customer_id,items,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, q.ID, t.OrganizationID, conversationID, customer, raw, q.ExpiresAt)
	if e != nil {
		return Quote{}, tenancy.Error(e)
	}
	if e = commit(ctx, tx); e != nil {
		return Quote{}, e
	}
	return q, nil
}

// Consent comes from an authenticated, explicit confirmation endpoint. LLM tool
// arguments never establish Accepted. The quote is scoped to this exact chat.
type Consent struct {
	QuoteID        string
	ConversationID string
	IdempotencyKey string
	Accepted       bool
}

func (s Service) CreateRequest(ctx context.Context, t catalog.Tenant, c Consent) (json.RawMessage, error) {
	if !c.Accepted {
		return nil, tenancy.Failure(422, "CONSENT_REQUIRED", "Нужно явное подтверждение показанного предложения")
	}
	if !organizations.ValidID(c.QuoteID) || !organizations.ValidID(c.IdempotencyKey) {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Некорректное предложение или ключ")
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if e = channelConsent(ctx, tx, t, c.ConversationID); e != nil {
		return nil, e
	}
	customer, e := conversation(ctx, tx, c.ConversationID)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, t.OrganizationID+":"+c.IdempotencyKey); e != nil {
		return nil, tenancy.Error(e)
	}
	var existing, quote, key, existingConversation string
	e = tx.QueryRow(ctx, `SELECT id,quote_id,idempotency_key,conversation_id FROM platform.orders WHERE request_kind='product_order' AND (idempotency_key=$1 OR quote_id=$2)`, c.IdempotencyKey, c.QuoteID).Scan(&existing, &quote, &key, &existingConversation)
	if e == nil {
		if quote != c.QuoteID || key != c.IdempotencyKey || existingConversation != c.ConversationID {
			return nil, failure("IDEMPOTENCY_CONFLICT", "Ключ уже связан с другим подтверждением")
		}
		result, e := detail(ctx, tx, existing)
		return result, e
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return nil, tenancy.Error(e)
	}
	var raw []byte
	var expires time.Time
	e = tx.QueryRow(ctx, `SELECT items,expires_at FROM platform.order_quotes WHERE id=$1 AND conversation_id=$2 AND customer_id=$3 AND agreed_at IS NULL FOR UPDATE`, c.QuoteID, c.ConversationID, customer).Scan(&raw, &expires)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if !expires.After(time.Now()) {
		return nil, failure("QUOTE_EXPIRED", "Предложение истекло. Обновите цену и подтвердите снова")
	}
	var snapshots []Snapshot
	if e = json.Unmarshal(raw, &snapshots); e != nil {
		return nil, auth.Unavailable()
	}
	cart := []CartItem{}
	for _, i := range snapshots {
		cart = append(cart, CartItem{i.VariantID, i.Quantity})
	}
	current, e := s.lockCatalog(ctx, tx, cart, true)
	if e != nil {
		return nil, e
	}
	var total int64
	for index, i := range current {
		old := snapshots[index]
		if i.UnitPrice != old.UnitPrice || i.Currency != old.Currency || i.VariantVersion != old.VariantVersion {
			return nil, failure("PRICE_CHANGED", "Цена или характеристики изменились. Обновите предложение и подтвердите снова")
		}
		if i.Available < i.Quantity {
			return nil, failure("INSUFFICIENT_STOCK", "Доступное количество изменилось. Обновите предложение")
		}
		total += old.UnitPrice * int64(old.Quantity)
	}
	id := auth.UUID()
	_, e = tx.Exec(ctx, `UPDATE platform.order_quotes SET agreed_at=now(),agreed_by=$2 WHERE id=$1`, c.QuoteID, actor(t))
	if e != nil {
		return nil, tenancy.Error(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform.orders(id,organization_id,conversation_id,customer_id,quote_id,idempotency_key,total_amount) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, t.OrganizationID, c.ConversationID, customer, c.QuoteID, c.IdempotencyKey, total)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	for _, i := range snapshots {
		_, e = tx.Exec(ctx, `INSERT INTO platform.order_items(id,organization_id,order_id,variant_id,product_id,quantity,unit_price_amount,name_snapshot,sku_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, auth.UUID(), t.OrganizationID, id, i.VariantID, i.ProductID, i.Quantity, i.UnitPrice, i.Name, i.SKU)
		if e != nil {
			return nil, tenancy.Error(e)
		}
	}
	result, e := detail(ctx, tx, id)
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform.order_events(id,organization_id,order_id,actor_id,kind,idempotency_key,request_hash,result) VALUES($1,$2,$3,$4,'created',$5,$6,$7)`, auth.UUID(), t.OrganizationID, id, actor(t), c.IdempotencyKey, auth.Hash(c.QuoteID), []byte(result))
	if e != nil {
		return nil, tenancy.Error(e)
	}
	result, e = detail(ctx, tx, id)
	if e != nil {
		return nil, e
	}
	if e = commit(ctx, tx); e != nil {
		return nil, e
	}
	return result, nil
}
func (s Service) Get(ctx context.Context, t catalog.Tenant, id string) (json.RawMessage, error) {
	if !organizations.ValidID(id) {
		return nil, tenancy.Failure(404, "NOT_FOUND", "Заявка не найдена")
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	return detail(ctx, tx, id)
}
func (s Service) GetForConversation(ctx context.Context, t catalog.Tenant, id, conversationID string) (json.RawMessage, error) {
	result, e := s.Get(ctx, t, id)
	if e != nil {
		return nil, e
	}
	var header struct {
		ConversationID string `json:"conversation_id"`
	}
	_ = json.Unmarshal(result, &header)
	if header.ConversationID != conversationID {
		return nil, tenancy.Failure(404, "NOT_FOUND", "Заявка не найдена")
	}
	return result, nil
}
func (s Service) List(ctx context.Context, t catalog.Tenant, status string, page int) (json.RawMessage, error) {
	if page < 1 || page > 10000 {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Некорректная страница")
	}
	if status != "" && !strings.Contains("|pending_confirmation|confirmed|rejected|cancelled|completed|", "|"+status+"|") {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Некорректный статус")
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	return readJSON(ctx, tx, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(CASE WHEN x.request_kind='product_order' THEN to_jsonb(x)-'request_kind'-'request_details'-'listing_id' ELSE to_jsonb(x) END ORDER BY x.created_at DESC,x.id),'[]'::jsonb)) FROM (SELECT * FROM platform.orders WHERE ($1='' OR status=$1) ORDER BY created_at DESC,id LIMIT 20 OFFSET $2) x`, status, (page-1)*20)
}
func (s Service) Change(ctx context.Context, t catalog.Tenant, id, action string, in Transition) (json.RawMessage, error) {
	if t.ChannelCapability != "" || t.AgentLease != "" || t.AgentTaskID != "" {
		return nil, tenancy.Failure(403, "FORBIDDEN", "Only staff can change order status")
	}
	target := map[string]string{"confirm": "confirmed", "reject": "rejected", "cancel": "cancelled", "complete": "completed"}[action]
	if target == "" || !organizations.ValidID(id) || !organizations.ValidID(in.IdempotencyKey) || in.ExpectedVersion < 1 || len([]rune(in.Reason)) > 500 || strings.ContainsRune(in.Reason, 0) {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Некорректная команда заявки")
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var status, requestKind string
	var requestDetails []byte
	var version int64
	e = tx.QueryRow(ctx, `SELECT status,version,request_kind,request_details FROM platform.orders WHERE id=$1 FOR UPDATE`, id).Scan(&status, &version, &requestKind, &requestDetails)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	fingerprint, _ := json.Marshal(struct {
		Action string
		Input  Transition
	}{action, in})
	hash := auth.Hash(string(fingerprint))
	var savedHash, saved []byte
	e = tx.QueryRow(ctx, `SELECT request_hash,result FROM platform.order_events WHERE order_id=$1 AND idempotency_key=$2`, id, in.IdempotencyKey).Scan(&savedHash, &saved)
	if e == nil {
		if !equal(hash, savedHash) {
			return nil, failure("IDEMPOTENCY_CONFLICT", "Ключ использован для другой команды")
		}
		return json.RawMessage(saved), nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return nil, tenancy.Error(e)
	}
	if version != in.ExpectedVersion {
		return nil, failure("VERSION_CONFLICT", "Заявка изменилась. Обновите страницу")
	}
	allowed := status == "pending_confirmation" && (target == "confirmed" || target == "rejected" || target == "cancelled") || status == "confirmed" && (target == "cancelled" || target == "completed")
	if !allowed {
		return nil, failure("INVALID_STATE", "Этот переход статуса запрещён")
	}
	if requestKind == "service_booking" {
		if e = checkAssignedTransition(ctx, tx, t.OrganizationID, id, target); e != nil {
			return nil, e
		}
	}
	if requestKind == "product_order" {
		if e = s.changeProductStock(ctx, tx, t, id, target, status); e != nil {
			return nil, e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE platform.orders SET status=$2,version=version+1,updated_at=now(),confirmed_at=CASE WHEN $2='confirmed' THEN now() ELSE confirmed_at END,confirmed_by=CASE WHEN $2='confirmed' THEN $3::uuid ELSE confirmed_by END WHERE id=$1`, id, target, actor(t))
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if requestKind != "product_order" {
		if e = s.changeIntake(ctx, tx, t, id, requestKind, target, status, requestDetails); e != nil {
			return nil, e
		}
	}
	result, e := detail(ctx, tx, id)
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform.order_events(id,organization_id,order_id,actor_id,kind,reason,idempotency_key,request_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, auth.UUID(), t.OrganizationID, id, actor(t), target, in.Reason, in.IdempotencyKey, hash, []byte(result))
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if e = commit(ctx, tx); e != nil {
		return nil, e
	}
	return result, nil
}
func (s Service) changeProductStock(ctx context.Context, tx pgx.Tx, t catalog.Tenant, id, target, status string) error {
	rows, e := tx.Query(ctx, `SELECT variant_id,quantity,unit_price_amount FROM platform.order_items WHERE order_id=$1 ORDER BY variant_id`, id)
	if e != nil {
		return tenancy.Error(e)
	}
	cart := []CartItem{}
	prices := map[string]int64{}
	for rows.Next() {
		var i CartItem
		var price int64
		if e = rows.Scan(&i.VariantID, &i.Quantity, &price); e != nil {
			rows.Close()
			return tenancy.Error(e)
		}
		cart = append(cart, i)
		prices[i.VariantID] = price
	}
	rows.Close()
	if rows.Err() != nil {
		return tenancy.Error(rows.Err())
	}
	kind := ""
	if target == "confirmed" {
		kind = "reserve"
	} else if status == "confirmed" && target == "cancelled" {
		kind = "release"
	} else if target == "completed" {
		kind = "consume"
	}
	if kind != "" {
		current, e := s.lockCatalog(ctx, tx, cart, target == "confirmed")
		if e != nil {
			return e
		}
		for _, i := range current {
			if target == "confirmed" {
				if i.UnitPrice != prices[i.VariantID] {
					return failure("PRICE_CHANGED", "Цена изменилась. Нужно новое предложение и согласие покупателя")
				}
				if !i.Fresh {
					return failure("STALE_INVENTORY", "Остаток требует ручной сверки магазина")
				}
				if i.Available < i.Quantity {
					return failure("INSUFFICIENT_STOCK", "Недостаточно товара для резерва")
				}
			}
			if e = catalog.MoveOrderStock(ctx, tx, t, id, i.VariantID, kind, i.Quantity); e != nil {
				return e
			}
		}
	}
	return nil
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func actor(t catalog.Tenant) any {
	if t.ChannelCapability != "" {
		return nil
	}
	return t.Session.User.ID
}

func channelConsent(ctx context.Context, tx pgx.Tx, t catalog.Tenant, id string) error {
	var channel, mode string
	if !organizations.ValidID(id) {
		return tenancy.Failure(404, "NOT_FOUND", "Диалог не найден")
	}
	if e := tx.QueryRow(ctx, `SELECT channel,control_mode FROM platform.conversations WHERE id=$1 FOR UPDATE`, id).Scan(&channel, &mode); e != nil {
		return tenancy.Error(e)
	}
	if channel != "internal_chat" && t.ChannelCapability != "" && mode != "agent" {
		return failure("HUMAN_CONTROL", "Диалог передан сотруднику")
	}
	if channel != "internal_chat" && t.ChannelCapability == "" {
		return tenancy.Failure(403, "CUSTOMER_PRINCIPAL_REQUIRED", "Telegram consent requires verified customer callback")
	}
	return nil
}
