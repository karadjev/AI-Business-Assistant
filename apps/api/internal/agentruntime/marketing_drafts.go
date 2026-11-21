package agentruntime

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type MarketingDraftAction struct {
	Version int     `json:"version"`
	Action  string  `json:"action"`
	Content *string `json:"content,omitempty"`
	Reason  *string `json:"reason,omitempty"`
}

func (s Service) ListMarketingDrafts(ctx context.Context, tenant catalog.Tenant) (json.RawMessage, error) {
	return s.list(ctx, tenant, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(d ORDER BY d.created_at DESC,d.id,d.version DESC),'[]'))
	FROM (SELECT id,organization_id,source_execution_id,version,created_at,goal,audience,channel,content,status,reason,decided_at
	FROM platform.marketing_drafts WHERE organization_id=$1 ORDER BY created_at DESC,id,version DESC LIMIT 50) d`)
}

func (s Service) ChangeMarketingDraft(ctx context.Context, tenant catalog.Tenant, id string, in MarketingDraftAction) error {
	validText := func(value *string, limit int) bool {
		return value != nil && strings.TrimSpace(*value) != "" && utf8.RuneCountInString(*value) <= limit &&
			!strings.ContainsRune(*value, 0)
	}
	valid := false
	switch in.Action {
	case "approved":
		valid = in.Content == nil && in.Reason == nil
	case "rejected":
		valid = in.Content == nil && (in.Reason == nil || validText(in.Reason, 500))
	case "revise":
		valid = in.Reason == nil && validText(in.Content, 8000)
	}
	if !valid || !organizations.ValidID(id) || in.Version < 1 || in.Version > 2147483647 {
		return auth.Failure(422, "INVALID_DRAFT_ACTION", "Проверьте параметры черновика")
	}
	tx, err := s.ownerTx(ctx, tenant)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var changed bool
	err = tx.QueryRow(ctx, "SELECT platform.marketing_draft_action($1,$2,$3,$4,$5,$6)",
		tenant.OrganizationID, id, in.Version, in.Action, in.Content, in.Reason).Scan(&changed)
	if err != nil {
		return tenancy.Error(err)
	}
	if !changed {
		return auth.Failure(409, "DRAFT_CONFLICT", "Версия или состояние черновика изменились")
	}
	if err := tx.Commit(ctx); err != nil {
		return tenancy.Error(err)
	}
	return nil
}
