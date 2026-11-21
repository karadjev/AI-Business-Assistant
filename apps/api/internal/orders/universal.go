package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type Intake struct {
	ConversationID string        `json:"conversation_id"`
	IdempotencyKey string        `json:"idempotency_key"`
	Kind           string        `json:"kind"`
	Details        IntakeDetails `json:"details"`
}

type IntakeDetails struct {
	Vehicle    *catalog.FitmentVehicle   `json:"vehicle,omitempty"`
	Summary    string                    `json:"summary,omitempty"`
	OfferingID string                    `json:"offering_id,omitempty"`
	ListingID  string                    `json:"listing_id,omitempty"`
	Selection  *servicebooking.Selection `json:"selection,omitempty"`
	Items      []CartItem                `json:"items,omitempty"`
}

func invalidIntake() error {
	return tenancy.Failure(422, "INVALID_REQUEST", "Проверьте обязательные параметры заявки")
}

func (in *Intake) validate() error {
	if !organizations.ValidID(in.ConversationID) || !organizations.ValidID(in.IdempotencyKey) {
		return invalidIntake()
	}
	d := &in.Details
	d.Summary = strings.TrimSpace(d.Summary)
	if !utf8.ValidString(d.Summary) || utf8.RuneCountInString(d.Summary) > 500 || strings.ContainsFunc(d.Summary, unicode.IsControl) {
		return invalidIntake()
	}
	hasOffering, hasListing, hasSlot, hasItems := d.OfferingID != "", d.ListingID != "", d.Selection != nil, len(d.Items) > 0
	if (hasOffering && !organizations.ValidID(d.OfferingID)) || (hasListing && !organizations.ValidID(d.ListingID)) {
		return invalidIntake()
	}
	valid := false
	switch in.Kind {
	case "service_booking":
		valid = hasSlot && !hasListing && !hasItems && !hasOffering
	case "repair_request":
		valid = hasOffering && !hasListing && !hasSlot && !hasItems && d.Summary != ""
	case "quotation_request", "custom_order":
		valid = !hasOffering && !hasListing && !hasSlot && !hasItems && d.Summary != ""
	case "property_viewing_request":
		valid = hasListing && !hasOffering && !hasItems && (!hasSlot || organizations.ValidID(d.Selection.OfferingID))
	case "hybrid_order":
		valid = hasOffering != hasSlot && hasItems && !hasListing && validCart(d.Items) == nil
	}
	if v := d.Vehicle; v != nil && ((in.Kind != "service_booking" && in.Kind != "hybrid_order") || v.Year < 1886 || v.Year > 2100 ||
		strings.TrimSpace(v.Make) == "" || strings.TrimSpace(v.Model) == "" || len(v.Make) > 80 || len(v.Model) > 80 || len(v.Engine) > 80 ||
		strings.ContainsFunc(v.Make+v.Model+v.Engine, unicode.IsControl)) {
		return invalidIntake()
	}
	if !valid {
		return invalidIntake()
	}
	return nil
}

func kindAllowed(ctx context.Context, tx pgx.Tx, org, kind string) error {
	var model string
	if err := tx.QueryRow(ctx, `SELECT business_model FROM platform.order_request_business_model($1) business_model WHERE business_model IS NOT NULL`, org).Scan(&model); err != nil {
		return tenancy.Error(err)
	}
	allowed := false
	switch kind {
	case "service_booking", "repair_request":
		allowed = model == "service" || model == "hybrid"
	case "property_viewing_request":
		allowed = model == "agency"
	case "hybrid_order":
		allowed = model == "hybrid"
	case "quotation_request", "custom_order":
		allowed = model == "trade" || model == "service" || model == "agency" || model == "hybrid"
	}
	if !allowed {
		return tenancy.Failure(403, "BUSINESS_CAPABILITY_REQUIRED", "Тип заявки недоступен для этого бизнеса")
	}
	return nil
}

// PrepareIntake enters the existing order state machine. The request carries no
// financial commitment. A product order still requires a priced quote and consent.
func (s Service) PrepareIntake(ctx context.Context, t catalog.Tenant, in Intake) (json.RawMessage, error) {
	return s.prepareIntake(ctx, t, in, false)
}

// PrepareServiceDelegation allows a staff-prepared proposal for a channel
// conversation only when its exact parameters match the completed delegation.
func (s Service) PrepareServiceDelegation(ctx context.Context, t catalog.Tenant, in Intake) (json.RawMessage, error) {
	return s.prepareIntake(ctx, t, in, true)
}

func (s Service) prepareIntake(ctx context.Context, t catalog.Tenant, in Intake, delegated bool) (json.RawMessage, error) {
	if t.ChannelCapability != "" || t.AgentLease != "" || t.AgentTaskID != "" {
		return nil, tenancy.Failure(403, "FORBIDDEN", "Оформление заявки доступно сотруднику")
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(in.Details)
	if err != nil {
		return nil, invalidIntake()
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || $2,25020))`, t.OrganizationID, in.IdempotencyKey); err != nil {
		return nil, tenancy.Error(err)
	}
	var id, kind, conv string
	var identical bool
	err = tx.QueryRow(ctx, `SELECT id,request_kind,conversation_id,coalesce(request_details=$3::jsonb,false)
	 FROM platform.orders WHERE organization_id=$1 AND idempotency_key=$2`, t.OrganizationID, in.IdempotencyKey, payload).Scan(&id, &kind, &conv, &identical)
	if err == nil {
		if !identical || kind != in.Kind || conv != in.ConversationID {
			return nil, failure("IDEMPOTENCY_CONFLICT", "Ключ уже связан с другой заявкой")
		}
		return detail(ctx, tx, id)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, tenancy.Error(err)
	}
	if err = kindAllowed(ctx, tx, t.OrganizationID, in.Kind); err != nil {
		return nil, err
	}
	customer, err := conversation(ctx, tx, in.ConversationID)
	if err != nil {
		return nil, err
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT c.channel='internal_chat' OR ($3::boolean
	 AND c.channel IN ('telegram','whatsapp','instagram') AND c.status='open' AND c.expires_at>clock_timestamp()
	 AND $4::text IN ('service_booking','hybrid_order') AND EXISTS(
	  SELECT 1 FROM platform.agent_tasks j JOIN platform.agent_executions e
	  ON e.organization_id=j.organization_id AND e.task_id=j.id AND e.status='completed'
	  WHERE j.organization_id=c.organization_id AND j.conversation_id=c.id AND j.id=$5
	  AND j.action='delegate_service' AND j.status='completed' AND j.deadline>clock_timestamp()
	  AND e.result->0->>'state'='approval_required' AND e.result->0->'proposal'->>'kind'=$4
	  AND e.result->0->'proposal'->'details'=$6::jsonb))
	 FROM platform.conversations c WHERE c.organization_id=$1 AND c.id=$2`,
		t.OrganizationID, in.ConversationID, delegated, in.Kind, in.IdempotencyKey, payload).Scan(&allowed); err != nil {
		return nil, tenancy.Error(err)
	}
	if !allowed {
		return nil, failure("UNSUPPORTED_CONVERSATION", "Для этого типа заявки нужен внутренний диалог")
	}
	if err = s.validateIntakeDetails(ctx, tx, t.OrganizationID, in.Kind, in.Details); err != nil {
		return nil, err
	}
	id = auth.UUID()
	_, err = tx.Exec(ctx, `INSERT INTO platform.orders(id,organization_id,conversation_id,customer_id,idempotency_key,request_kind,request_details,listing_id)
	 VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,'')::uuid)`, id, t.OrganizationID, in.ConversationID, customer, in.IdempotencyKey, in.Kind, payload, in.Details.ListingID)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	if in.Kind == "hybrid_order" && in.Details.Selection != nil {
		if err = s.saveHybridItems(ctx, tx, t, id, in.Details.Items); err != nil {
			return nil, err
		}
	}
	result, err := detail(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	hash := auth.Hash(in.Kind + ":" + in.ConversationID + ":" + string(payload))
	_, err = tx.Exec(ctx, `INSERT INTO platform.order_events(id,organization_id,order_id,actor_id,kind,idempotency_key,request_hash,result)
	 VALUES($1,$2,$3,$4,'created',$5,$6,$7)`, auth.UUID(), t.OrganizationID, id, actor(t), in.IdempotencyKey, hash, []byte(result))
	if err != nil {
		return nil, tenancy.Error(err)
	}
	if err = commit(ctx, tx); err != nil {
		return nil, err
	}
	return result, nil
}

func (s Service) validateIntakeDetails(ctx context.Context, tx pgx.Tx, org, kind string, d IntakeDetails) error {
	switch kind {
	case "service_booking":
		if d.Selection == nil {
			return invalidIntake()
		}
		return servicebooking.ValidateSelectionTx(ctx, tx, org, *d.Selection)
	case "repair_request", "hybrid_order":
		if d.Selection != nil {
			// Snapshot inventory first. Booking mutations always lock stock before service resources.
			items, err := s.lockCatalog(ctx, tx, d.Items, true)
			if err != nil {
				return err
			}
			if err := checkHybridAvailability(items); err != nil {
				return err
			}
			return servicebooking.ValidateSelectionTx(ctx, tx, org, *d.Selection)
		}
		if err := servicebooking.ValidateOfferingTx(ctx, tx, org, d.OfferingID); err != nil {
			return err
		}
		if kind == "hybrid_order" {
			items, err := s.lockCatalog(ctx, tx, d.Items, true)
			if err != nil {
				return err
			}
			return checkHybridAvailability(items)
		}
	case "property_viewing_request":
		var status string
		var until time.Time
		err := tx.QueryRow(ctx, `SELECT status,verified_until FROM platform.property_listings WHERE organization_id=$1 AND id=$2`, org, d.ListingID).Scan(&status, &until)
		if err != nil {
			return tenancy.Error(err)
		}
		if status != "available" || !until.After(time.Now()) {
			return failure("LISTING_UNVERIFIED", "Актуальность объекта не подтверждена")
		}
		if d.Selection != nil {
			var matches bool
			err = tx.QueryRow(ctx, `SELECT platform.property_viewing_matches($1,$2,$3)`, org, d.ListingID, d.Selection.OfferingID).Scan(&matches)
			if err != nil {
				return tenancy.Error(err)
			}
			if !matches {
				return failure("VIEWING_UNAVAILABLE", "Время просмотра для объекта не подтверждено")
			}
			return servicebooking.ValidateSelectionTx(ctx, tx, org, *d.Selection)
		}
	}
	return nil
}

func (s Service) changeIntake(ctx context.Context, tx pgx.Tx, t catalog.Tenant, id, kind, target, status string, raw []byte) error {
	var d IntakeDetails
	if err := json.Unmarshal(raw, &d); err != nil {
		return auth.Unavailable()
	}
	org := t.OrganizationID
	if kind == "hybrid_order" && d.Selection != nil {
		return s.changeHybrid(ctx, tx, t, id, target, status, *d.Selection)
	}
	if target == "confirmed" {
		if err := kindAllowed(ctx, tx, org, kind); err != nil {
			return err
		}
		if kind == "service_booking" {
			if d.Selection == nil {
				return auth.Unavailable()
			}
			return servicebooking.ConfirmTx(ctx, tx, org, id, *d.Selection)
		}
		if kind == "property_viewing_request" && d.Selection != nil {
			if err := s.validateIntakeDetails(ctx, tx, org, kind, d); err != nil {
				return err
			}
			return servicebooking.ConfirmTx(ctx, tx, org, id, *d.Selection)
		}
		return s.validateIntakeDetails(ctx, tx, org, kind, d)
	}
	if kind == "service_booking" || (kind == "property_viewing_request" && d.Selection != nil) {
		if target == "cancelled" && status == "confirmed" {
			return servicebooking.CancelTx(ctx, tx, org, id)
		}
		if target == "completed" {
			var ended bool
			err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.service_bookings WHERE organization_id=$1 AND request_id=$2
				AND status='confirmed' AND ends_at <= clock_timestamp())`, org, id).Scan(&ended)
			if err != nil {
				return tenancy.Error(err)
			}
			if !ended {
				return failure("SERVICE_NOT_ENDED", "Запись ещё не завершилась")
			}
		}
	}
	return nil
}

type PropertyFilter struct {
	Rooms        *int   `json:"rooms,omitempty"`
	MinArea      *int   `json:"min_area_m2,omitempty"`
	MaxArea      *int   `json:"max_area_m2,omitempty"`
	MinPrice     *int64 `json:"min_price_amount,omitempty"`
	MaxPrice     *int64 `json:"max_price_amount,omitempty"`
	Locality     string `json:"locality,omitempty"`
	PropertyType string `json:"property_type,omitempty"`
	Status       string `json:"status,omitempty"`
	Page         int    `json:"page,omitempty"`
}

func (f PropertyFilter) Valid() bool {
	if f.Page < 0 || f.Page > 100 || (f.Rooms != nil && (*f.Rooms < 0 || *f.Rooms > 50)) ||
		(f.MinArea != nil && (*f.MinArea < 1 || *f.MinArea > 1000000)) ||
		(f.MaxArea != nil && (*f.MaxArea < 1 || *f.MaxArea > 1000000)) ||
		(f.MinArea != nil && f.MaxArea != nil && *f.MinArea > *f.MaxArea) ||
		(f.MinPrice != nil && *f.MinPrice < 0) || (f.MaxPrice != nil && *f.MaxPrice < 0) ||
		(f.MinPrice != nil && f.MaxPrice != nil && *f.MinPrice > *f.MaxPrice) ||
		!utf8.ValidString(f.Locality) || utf8.RuneCountInString(f.Locality) > 120 || strings.ContainsFunc(f.Locality, unicode.IsControl) {
		return false
	}
	switch f.PropertyType {
	case "", "apartment", "house", "commercial", "land", "other":
	default:
		return false
	}
	switch f.Status {
	case "", "available", "unavailable", "unknown":
		return true
	default:
		return false
	}
}

func (s Service) SearchProperties(ctx context.Context, t catalog.Tenant, f PropertyFilter) (json.RawMessage, error) {
	if !f.Valid() {
		return nil, invalidIntake()
	}
	if f.Page == 0 {
		f.Page = 1
	}
	if t.ChannelCapability != "" || t.AgentLease != "" {
		return nil, tenancy.Failure(403, "FORBIDDEN", "Поиск объектов доступен сотруднику")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = kindAllowed(ctx, tx, t.OrganizationID, "property_viewing_request"); err != nil {
		return nil, err
	}
	return readJSON(ctx, tx, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(jsonb_strip_nulls(to_jsonb(p)) ORDER BY p.rooms,p.area_m2,p.id),'[]'))
	 FROM (
	   SELECT * FROM (
	     SELECT id,title,locality,rooms,area_m2,price_amount,currency_code,property_type,viewing_offering_id,
	       CASE WHEN status='available' AND verified_until<=statement_timestamp() THEN 'unknown' ELSE status END AS status,verified_until
	     FROM platform.property_listings
	     WHERE organization_id=$1 AND ($2::int IS NULL OR rooms=$2)
	       AND ($3::int IS NULL OR area_m2 >= $3) AND ($4::int IS NULL OR area_m2 <= $4)
	       AND ($5::bigint IS NULL OR price_amount >= $5) AND ($6::bigint IS NULL OR price_amount <= $6)
	       AND ($7='' OR lower(locality)=lower($7)) AND ($8='' OR property_type=$8)
	   ) listings
	   WHERE ($9='' OR status=$9)
	   ORDER BY rooms,area_m2,id LIMIT 20 OFFSET $10
	 ) p`,
		t.OrganizationID, f.Rooms, f.MinArea, f.MaxArea, f.MinPrice, f.MaxPrice, strings.TrimSpace(f.Locality), f.PropertyType, f.Status, (f.Page-1)*20)
}

func (s Service) CheckConstruction(ctx context.Context, t catalog.Tenant) error {
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var yes bool
	err = tx.QueryRow(ctx, `SELECT industry_profile='building_materials' FROM platform.organizations WHERE id=$1`, t.OrganizationID).Scan(&yes)
	if err != nil {
		return tenancy.Error(err)
	}
	if !yes {
		return tenancy.Failure(403, "BUSINESS_CAPABILITY_REQUIRED", "Расчёт недоступен для этого профиля")
	}
	return nil
}
