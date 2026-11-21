package instagram

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

// Service uses the shared channel repository. This runtime is synthetic-only;
// no provider client or outbound network transport is constructed here.
type Service struct {
	Pool   *pgxpool.Pool
	Config config.Instagram
	Chat   *conversations.Service
	Log    *slog.Logger
}

func (s Service) Proof() []byte {
	h := hmac.New(sha256.New, s.Config.Key)
	h.Write([]byte("aba:instagram:worker:v1"))
	return h.Sum(nil)
}
func (s Service) begin(ctx context.Context, t catalog.Tenant) (pgx.Tx, error) {
	if t.ChannelCapability != "" {
		return nil, auth.Failure(403, "FORBIDDEN", "Требуется владелец магазина")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return nil, err
	}
	var owner bool
	if err = tx.QueryRow(ctx, `SELECT platform.instagram_owner($1)`, t.OrganizationID).Scan(&owner); err != nil || !owner {
		tx.Rollback(ctx)
		return nil, auth.Failure(404, "NOT_FOUND", "Подключение не найдено")
	}
	return tx, nil
}
func (s Service) Status(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, err := s.begin(ctx, t)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT jsonb_build_object('mode','mock','live_enabled',false,'runtime_enabled',$2::boolean,'sending_enabled',false,'connection',(SELECT to_jsonb(c)||jsonb_build_object('status',CASE WHEN c.revoked_at IS NOT NULL THEN 'not_connected' WHEN c.status IN ('webhook_error','reauthorization_required') THEN c.status WHEN c.status='permission_error' OR (c.token_expires_at IS NOT NULL AND NOT c.permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages']) THEN 'permission_error' WHEN (c.token_expires_at IS NULL AND c.oauth_expires_at<=clock_timestamp()) OR (c.token_expires_at IS NOT NULL AND c.token_expires_at<=clock_timestamp()) THEN 'reauthorization_required' WHEN NOT c.webhook_subscribed AND c.token_expires_at IS NOT NULL THEN 'webhook_error' WHEN c.webhook_seen_at IS NULL THEN 'connecting' ELSE c.status END) FROM platform.instagram_connections c WHERE organization_id=$1),'queue',jsonb_build_object('pending',(SELECT count(*) FROM platform.channel_inbox WHERE organization_id=$1 AND channel='instagram' AND status IN ('pending','processing')),'dead_letter',(SELECT count(*) FROM platform.channel_inbox WHERE organization_id=$1 AND channel='instagram' AND status='failed')))`, t.OrganizationID, s.Config.Enabled).Scan(&raw)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	return raw, nil
}

// stateStore lives only within one owner-authorized transaction. State is never
// stored in memory or trusted from tenant fields in callback payloads.
type stateStore struct {
	tx          pgx.Tx
	tenant      catalog.Tenant
	integration string
	consumed    bool
}

func (s *stateStore) Create(ctx context.Context, h [32]byte, b StateBinding, expiry time.Time) error {
	if b.OrganizationID != s.tenant.OrganizationID || b.UserID != s.tenant.Session.User.ID || b.SessionID != hex.EncodeToString(s.tenant.Session.Hash) {
		return ErrRejected
	}
	_, err := s.tx.Exec(ctx, `SELECT platform.instagram_state_create($1,$2,$3,$4,$5,$6)`, b.OrganizationID, s.integration, h[:], s.tenant.Session.Hash, b.RedirectURI, expiry)
	return err
}
func (s *stateStore) Consume(ctx context.Context, h [32]byte, b StateBinding, _ time.Time) error {
	if b.OrganizationID != s.tenant.OrganizationID || b.UserID != s.tenant.Session.User.ID || b.SessionID != hex.EncodeToString(s.tenant.Session.Hash) {
		return ErrRejected
	}
	var ok bool
	err := s.tx.QueryRow(ctx, `SELECT platform.instagram_state_consume($1,$2,$3,$4)`, b.OrganizationID, h[:], s.tenant.Session.Hash, b.RedirectURI).Scan(&ok)
	if err != nil || !ok {
		return ErrRejected
	}
	s.consumed = true
	return nil
}
func redirectURI(org string) string {
	return "https://avero.example.test/api/v1/organizations/" + org + "/integrations/instagram/oauth/callback"
}
func oauthBinding(t catalog.Tenant) StateBinding {
	return StateBinding{t.OrganizationID, t.Session.User.ID, hex.EncodeToString(t.Session.Hash), redirectURI(t.OrganizationID)}
}
func syntheticAccount(org string) string {
	h := sha256.Sum256([]byte("instagram/mock/" + org))
	return fmt.Sprintf("8%017d", binary.BigEndian.Uint64(h[:8])%100000000000000000)
}
func (s Service) enabled() bool { return s.Config.Enabled && len(s.Config.Key) == 32 }
func (s Service) Start(ctx context.Context, t catalog.Tenant) (map[string]any, error) {
	tx, err := s.begin(ctx, t)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if !s.enabled() {
		return nil, auth.Failure(409, "INSTAGRAM_DISABLED", "Тестовый Instagram не настроен; live выключен")
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,8217))`, t.OrganizationID); err != nil {
		return nil, tenancy.Error(err)
	}
	b := Binding{OrganizationID: t.OrganizationID, IntegrationID: auth.UUID(), AccountID: syntheticAccount(t.OrganizationID)}
	var existingID string
	err = tx.QueryRow(ctx, `SELECT id FROM platform.channel_integrations WHERE organization_id=$1 AND channel='instagram'`, t.OrganizationID).Scan(&existingID)
	if err == nil {
		b.IntegrationID = existingID
	} else if err != pgx.ErrNoRows {
		return nil, tenancy.Error(err)
	}
	cipher, err := (CredentialBox{Key: s.Config.Key}).Seal(b, []byte("mock:pending"))
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `SELECT platform.instagram_prepare($1,$2,$3,$4,$5)`, t.OrganizationID, b.IntegrationID, b.AccountID, cipher, auth.Hash(string(s.Proof()))); err != nil {
		return nil, tenancy.Error(err)
	}
	state := &stateStore{tx: tx, tenant: t, integration: b.IntegrationID}
	o := OAuth{AppID: "0", RedirectURI: redirectURI(t.OrganizationID), States: state}
	official, err := o.Start(ctx, oauthBinding(t), time.Now().UTC())
	if err != nil {
		return nil, auth.Unavailable()
	}
	u, _ := url.Parse(official)
	// The simulation deliberately never navigates to Meta or exchanges real codes.
	callback := "/api/v1/organizations/" + t.OrganizationID + "/integrations/instagram/oauth/callback?" + url.Values{"state": {u.Query().Get("state")}, "code": {"mock:authorized"}}.Encode()
	if err = tx.Commit(ctx); err != nil {
		return nil, tenancy.Error(err)
	}
	return map[string]any{"mode": "mock", "live_enabled": false, "callback_url": callback, "integration_id": b.IntegrationID}, nil
}
func (s Service) Callback(ctx context.Context, t catalog.Tenant, q url.Values) error {
	tx, err := s.begin(ctx, t)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if !s.enabled() {
		return auth.Failure(409, "INSTAGRAM_DISABLED", "Тестовый Instagram выключен")
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,8217))`, t.OrganizationID); err != nil {
		return tenancy.Error(err)
	}
	var b Binding
	b.OrganizationID = t.OrganizationID
	if err = tx.QueryRow(ctx, `SELECT integration_id,account_id FROM platform.instagram_connections WHERE organization_id=$1 AND revoked_at IS NULL`, t.OrganizationID).Scan(&b.IntegrationID, &b.AccountID); err != nil {
		return tenancy.Error(err)
	}
	state := &stateStore{tx: tx, tenant: t, integration: b.IntegrationID}
	o := OAuth{AppID: "0", RedirectURI: redirectURI(t.OrganizationID), States: state}
	code, validation := o.Callback(ctx, oauthBinding(t), q, time.Now().UTC())
	if validation != nil || code != "mock:authorized" {
		if state.consumed {
			if _, err = tx.Exec(ctx, `SELECT platform.instagram_auth_failure($1,$2)`, t.OrganizationID, b.IntegrationID); err != nil {
				return tenancy.Error(err)
			}
		}
		// Persist valid-state consumption even for denial/bad code; never revive it on rollback.
		if err = tx.Commit(ctx); err != nil {
			return tenancy.Error(err)
		}
		return auth.Failure(403, "INSTAGRAM_OAUTH_REJECTED", "Авторизация отклонена или срок state истёк")
	}
	cipher, err := (CredentialBox{Key: s.Config.Key}).Seal(b, []byte("mock:account:"+b.AccountID))
	if err != nil {
		return err
	}
	// Synthetic provider confirms permissions/subscription, not delivery of an event.
	_, err = tx.Exec(ctx, `SELECT platform.instagram_authorize($1,$2,$3,$4,$5)`, t.OrganizationID, b.IntegrationID, cipher, time.Now().UTC().Add(60*24*time.Hour), []string{"instagram_business_basic", "instagram_business_manage_messages"})
	if err != nil {
		return tenancy.Error(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return tenancy.Error(err)
	}
	return nil
}
func (s Service) Disconnect(ctx context.Context, t catalog.Tenant) error {
	tx, err := s.begin(ctx, t)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT platform.instagram_disconnect($1)`, t.OrganizationID); err != nil {
		return tenancy.Error(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return tenancy.Error(err)
	}
	return nil
}
