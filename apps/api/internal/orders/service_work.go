package orders

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type StaffAssignment struct {
	OrderID    string `json:"order_id"`
	ResourceID string `json:"resource_id"`
	UserID     string `json:"user_id"`
}

type ServiceReceipt struct {
	OrderID       string `json:"order_id"`
	ReceivedMinor int64  `json:"received_minor"`
	CostMinor     *int64 `json:"cost_minor"`
}

func (s Service) AssignService(ctx context.Context, t catalog.Tenant, in StaffAssignment) (json.RawMessage, error) {
	if !organizations.ValidID(in.OrderID) || !organizations.ValidID(in.ResourceID) || !organizations.ValidID(in.UserID) {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Выберите заявку, ресурс и сотрудника")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = requireServiceOwner(ctx, tx, t.OrganizationID); err != nil {
		return nil, err
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM platform.orders WHERE organization_id=$1 AND id=$2
		AND request_kind='service_booking' FOR UPDATE`, t.OrganizationID, in.OrderID).Scan(&status)
	if err != nil {
		return nil, tenancy.Error(err)
	}

	var resource, user string
	err = tx.QueryRow(ctx, `SELECT resource_id,user_id FROM platform.service_assignments
		WHERE organization_id=$1 AND order_id=$2`, t.OrganizationID, in.OrderID).Scan(&resource, &user)
	if err == nil {
		if resource != in.ResourceID || user != in.UserID {
			return nil, failure("ALREADY_ASSIGNED", "У записи уже есть исполнитель. Для смены отмените запись и создайте новую")
		}
		return json.Marshal(in)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, tenancy.Error(err)
	}
	if status != "confirmed" {
		return nil, failure("INVALID_STATE", "Сначала подтвердите запись")
	}

	var booking string
	err = tx.QueryRow(ctx, `SELECT b.id FROM platform.service_bookings b
		JOIN platform.service_booking_allocations a ON a.organization_id=b.organization_id AND a.booking_id=b.id
		JOIN platform.service_resources r ON r.organization_id=a.organization_id AND r.id=a.resource_id
		WHERE b.organization_id=$1 AND b.request_id=$2 AND a.resource_id=$3 AND a.active
		AND b.status='confirmed' AND b.ends_at>clock_timestamp() AND r.active AND r.kind='staff' AND r.capacity=1 AND a.units=1
		FOR UPDATE OF r`, t.OrganizationID, in.OrderID, in.ResourceID).Scan(&booking)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, failure("STAFF_UNAVAILABLE", "Для будущей записи нужен выделенный ресурс сотрудника с вместимостью 1")
	}
	if err != nil {
		return nil, tenancy.Error(err)
	}
	var member bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jsonb_array_elements(platform.team_state($1)->'members') m
		WHERE m->>'id'=$2 AND m->>'operator'='true')`, t.OrganizationID, in.UserID).Scan(&member)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	if !member {
		return nil, tenancy.Failure(422, "INVALID_ASSIGNEE", "Сотруднику нужны права на заявки в этой организации")
	}

	// A booking already owns its exclusive, shift-validated resource including buffers.
	// The one-to-one binding extends that exclusion to the employee across offerings.
	_, err = tx.Exec(ctx, `INSERT INTO platform.service_staff_resources(organization_id,resource_id,user_id,bound_by)
		VALUES($1,$2,$3,platform.current_user_id()) ON CONFLICT DO NOTHING`, t.OrganizationID, in.ResourceID, in.UserID)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	var bound bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.service_staff_resources
		WHERE organization_id=$1 AND resource_id=$2 AND user_id=$3)`, t.OrganizationID, in.ResourceID, in.UserID).Scan(&bound)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	if !bound {
		return nil, failure("STAFF_RESOURCE_CONFLICT", "Сотрудник или ресурс уже закреплён за другой парой")
	}

	_, err = tx.Exec(ctx, `INSERT INTO platform.service_assignments(organization_id,order_id,booking_id,resource_id,user_id,assigned_by)
		VALUES($1,$2,$3,$4,$5,platform.current_user_id())`, t.OrganizationID, in.OrderID, booking, in.ResourceID, in.UserID)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	if err = commit(ctx, tx); err != nil {
		return nil, err
	}
	return json.Marshal(in)
}

func requireServiceOwner(ctx context.Context, tx pgx.Tx, org string) error {
	// Team changes take an exclusive lock on the organization.
	var locked string
	err := tx.QueryRow(ctx, `SELECT id FROM platform.organizations WHERE id=$1 AND platform.has_membership(id,true) FOR SHARE`, org).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenancy.Failure(403, "FORBIDDEN", "Действие доступно владельцу")
	}
	if err != nil {
		return tenancy.Error(err)
	}
	return nil
}

func checkAssignedTransition(ctx context.Context, tx pgx.Tx, org, order, target string) error {
	if target != "completed" && target != "cancelled" {
		return nil
	}
	var assigned, own, owner, hasStaffResource bool
	err := tx.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM platform.service_assignments WHERE organization_id=$1 AND order_id=$2),
		EXISTS(SELECT 1 FROM platform.service_assignments WHERE organization_id=$1 AND order_id=$2 AND user_id=platform.current_user_id()),
		platform.has_membership($1,true),
		EXISTS(SELECT 1 FROM platform.service_bookings b
		 JOIN platform.service_booking_allocations a ON a.organization_id=b.organization_id AND a.booking_id=b.id
		 JOIN platform.service_resources r ON r.organization_id=a.organization_id AND r.id=a.resource_id
		 WHERE b.organization_id=$1 AND b.request_id=$2 AND a.active AND r.kind='staff' AND r.capacity=1)`, org, order).
		Scan(&assigned, &own, &owner, &hasStaffResource)
	if err != nil {
		return tenancy.Error(err)
	}
	if target == "completed" && hasStaffResource && !assigned {
		return failure("ASSIGNMENT_REQUIRED", "Назначьте исполнителя перед завершением услуги")
	}
	if assigned && ((target == "completed" && !own) || (target == "cancelled" && !owner)) {
		return tenancy.Failure(403, "FORBIDDEN", "Назначенную услугу завершает исполнитель; отменить её может владелец")
	}
	return nil
}

func (s Service) RecordServiceReceipt(ctx context.Context, t catalog.Tenant, in ServiceReceipt) (json.RawMessage, error) {
	if !organizations.ValidID(in.OrderID) || in.ReceivedMinor < 1 || in.ReceivedMinor > 1_000_000_000_000 ||
		in.CostMinor == nil || *in.CostMinor < 0 || *in.CostMinor > 1_000_000_000_000 {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Укажите фактическое поступление и себестоимость в копейках")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = requireServiceOwner(ctx, tx, t.OrganizationID); err != nil {
		return nil, err
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT o.status FROM platform.orders o JOIN platform.service_assignments a
		ON a.organization_id=o.organization_id AND a.order_id=o.id
		WHERE o.organization_id=$1 AND o.id=$2 AND o.request_kind='service_booking' FOR UPDATE OF o`, t.OrganizationID, in.OrderID).Scan(&status)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	if status != "completed" {
		return nil, failure("INVALID_STATE", "Сначала исполнитель должен завершить услугу")
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform.service_receipts(organization_id,order_id,received_minor,cost_minor,recorded_by)
		VALUES($1,$2,$3,$4,platform.current_user_id()) ON CONFLICT(organization_id,order_id) DO NOTHING`, t.OrganizationID, in.OrderID, in.ReceivedMinor, *in.CostMinor)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	var matches bool
	err = tx.QueryRow(ctx, `SELECT received_minor=$3 AND cost_minor=$4 FROM platform.service_receipts
		WHERE organization_id=$1 AND order_id=$2`, t.OrganizationID, in.OrderID, in.ReceivedMinor, *in.CostMinor).Scan(&matches)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	if !matches {
		return nil, failure("RECEIPT_ALREADY_RECORDED", "Оплата уже учтена с другими суммами")
	}
	if err = commit(ctx, tx); err != nil {
		return nil, err
	}
	return json.Marshal(in)
}

func (s Service) ServiceSchedule(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	return readJSON(ctx, tx, `SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.starts_at), '[]') FROM (
		SELECT a.order_id,o.status,b.starts_at,b.ends_at,b.timezone,r.name AS resource_name
		FROM platform.service_assignments a
		JOIN platform.service_bookings b ON b.organization_id=a.organization_id AND b.id=a.booking_id
		JOIN platform.orders o ON o.organization_id=a.organization_id AND o.id=a.order_id
		JOIN platform.service_resources r ON r.organization_id=a.organization_id AND r.id=a.resource_id
		WHERE a.organization_id=$1 AND a.user_id=platform.current_user_id()
		ORDER BY (o.status='confirmed') DESC, CASE WHEN o.status='confirmed' THEN b.starts_at END, b.starts_at DESC LIMIT 100
		)x`, t.OrganizationID)
}

func (s Service) ServiceWorkState(ctx context.Context, t catalog.Tenant, id string) (json.RawMessage, error) {
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	return readJSON(ctx, tx, `SELECT jsonb_build_object(
		'assignment',(SELECT jsonb_build_object('user_id',a.user_id,'mine',a.user_id=platform.current_user_id(),'assigned_at',a.assigned_at,'name',r.name)
		  FROM platform.service_assignments a JOIN platform.service_resources r ON r.organization_id=a.organization_id AND r.id=a.resource_id
		  WHERE a.organization_id=o.organization_id AND a.order_id=o.id),
		'receipt',(SELECT jsonb_build_object('received_minor',p.received_minor,'cost_minor',p.cost_minor,'recorded_at',p.recorded_at)
		  FROM platform.service_receipts p WHERE p.organization_id=o.organization_id AND p.order_id=o.id),
		'resources',(SELECT coalesce(jsonb_agg(jsonb_build_object('id',r.id,'name',r.name,'user_id',sr.user_id) ORDER BY r.name),'[]')
		  FROM platform.service_bookings b
		  JOIN platform.service_booking_allocations a ON a.organization_id=b.organization_id AND a.booking_id=b.id
		  JOIN platform.service_resources r ON r.organization_id=a.organization_id AND r.id=a.resource_id
		  LEFT JOIN platform.service_staff_resources sr ON sr.organization_id=r.organization_id AND sr.resource_id=r.id
		  WHERE b.organization_id=o.organization_id AND b.request_id=o.id AND a.active AND r.kind='staff' AND r.capacity=1 AND r.active AND a.units=1))
		FROM platform.orders o WHERE o.organization_id=$1 AND o.id=$2 AND o.request_kind='service_booking'`, t.OrganizationID, id)
}

func (s Service) ServiceFinancials(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = requireServiceOwner(ctx, tx, t.OrganizationID); err != nil {
		return nil, err
	}
	return readJSON(ctx, tx, "SELECT platform.service_work_totals($1)", t.OrganizationID)
}
