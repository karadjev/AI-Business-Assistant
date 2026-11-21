package orders

import (
	"context"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type ViewingStatus string
type ViewingApproval string

type ViewingEmployee struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
}

type ViewingRequest struct {
	ID                string            `json:"id"`
	OrganizationID    string            `json:"organization_id"`
	CustomerID        string            `json:"customer_id"`
	PropertyID        string            `json:"property_id"`
	PreferredStart    *time.Time        `json:"preferred_start,omitempty"`
	Timezone          string            `json:"timezone,omitempty"`
	AssignedEmployees []ViewingEmployee `json:"assigned_employees"`
	CustomerComment   string            `json:"customer_comment,omitempty"`
	Status            ViewingStatus     `json:"status"`
	Approval          ViewingApproval   `json:"approval_status"`
	ApprovedBy        string            `json:"approved_by,omitempty"`
	Version           int64             `json:"version"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

func (s Service) GetPropertyViewing(ctx context.Context, t catalog.Tenant, id string) (ViewingRequest, error) {
	var result ViewingRequest
	if !organizations.ValidID(id) {
		return result, tenancy.Failure(404, "NOT_FOUND", "Заявка не найдена")
	}
	if t.ChannelCapability != "" || t.AgentTaskID != "" || t.AgentLease != "" {
		return result, tenancy.Failure(403, "FORBIDDEN", "Заявка доступна сотруднику")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT o.id,o.organization_id,o.customer_id,o.listing_id,
	 (o.request_details->'selection'->>'starts_at')::timestamptz,
	 coalesce(o.request_details->'selection'->>'timezone',''),coalesce(o.request_details->>'summary',''),
	 CASE o.status WHEN 'pending_confirmation' THEN 'pending' WHEN 'confirmed' THEN 'approved' ELSE o.status END,
	 CASE WHEN o.confirmed_by IS NOT NULL THEN 'approved'
	  WHEN o.status='rejected' THEN 'rejected' WHEN o.status='cancelled' THEN 'cancelled' ELSE 'pending' END,
	 coalesce(o.confirmed_by::text,''),o.version,o.created_at,o.updated_at,
	 coalesce((SELECT jsonb_agg(jsonb_build_object('resource_id',r.id,'name',r.name) ORDER BY r.id)
	  FROM platform.service_bookings b
	  JOIN platform.service_booking_allocations a ON (a.organization_id,a.booking_id)=(b.organization_id,b.id)
	  JOIN platform.service_resources r ON (r.organization_id,r.id)=(a.organization_id,a.resource_id)
	  WHERE (b.organization_id,b.request_id)=(o.organization_id,o.id) AND r.kind='staff' AND a.active),'[]')
	 FROM platform.orders o WHERE o.organization_id=$1 AND o.id=$2 AND o.request_kind='property_viewing_request'`,
		t.OrganizationID, id).Scan(&result.ID, &result.OrganizationID, &result.CustomerID, &result.PropertyID,
		&result.PreferredStart, &result.Timezone, &result.CustomerComment, &result.Status, &result.Approval,
		&result.ApprovedBy, &result.Version, &result.CreatedAt, &result.UpdatedAt, &result.AssignedEmployees)
	if err != nil {
		return result, tenancy.Error(err)
	}
	return result, nil
}
