package whatsapp

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"strconv"
	"time"
)

type Service struct {
	Pool      *pgxpool.Pool
	Key       []byte
	Chat      *conversations.Service
	Transport Transport
}

func (s Service) Proof() []byte {
	h := hmac.New(sha256.New, s.Key)
	h.Write([]byte("aba:whatsapp:worker:v1"))
	return h.Sum(nil)
}

type Credentials struct {
	AccessToken  string   `json:"access_token"`
	AppSecret    string   `json:"app_secret"`
	VerifyToken  string   `json:"verify_token"`
	AllowedUsers []string `json:"allowed_users"`
}
type ConnectInput struct {
	WindowSeconds int         `json:"window_seconds"`
	RetentionDays int         `json:"retention_days"`
	AppID         string      `json:"app_id"`
	BusinessID    string      `json:"business_account_id"`
	PhoneID       string      `json:"phone_number_id"`
	GraphVersion  string      `json:"graph_api_version"`
	Credentials   Credentials `json:"credentials"`
}

func seal(key []byte, org string, b []byte) ([]byte, error) {
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, channel.ErrRejected
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return nil, channel.ErrRejected
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return g.Seal(nonce, nonce, b, []byte(org+"/whatsapp/v1")), nil
}
func open(key []byte, org string, b []byte) ([]byte, error) {
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, channel.ErrRejected
	}
	g, e := cipher.NewGCM(block)
	if e != nil || len(b) < g.NonceSize() {
		return nil, channel.ErrRejected
	}
	raw, e := g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], []byte(org+"/whatsapp/v1"))
	if e != nil {
		return nil, channel.ErrRejected
	}
	return raw, nil
}
func (s Service) begin(ctx context.Context, t catalog.Tenant) (pgx.Tx, error) {
	return tenancy.Begin(ctx, s.Pool, t, true, true)
}
func (s Service) Status(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var b []byte
	e = tx.QueryRow(ctx, `SELECT jsonb_build_object('mode','mock','live_enabled',false,'monetary_budget',0,'inbox_pending',(SELECT count(*) FROM platform.channel_inbox WHERE channel='whatsapp' AND organization_id=$1 AND status IN ('pending','processing')),'outbox_pending',(SELECT count(*) FROM platform.channel_outbox WHERE integration_id IN (SELECT id FROM platform.channel_integrations WHERE channel='whatsapp') AND organization_id=$1 AND status IN ('pending','sending')),'delivery_unknown',(SELECT count(*) FROM platform.channel_outbox WHERE integration_id IN (SELECT id FROM platform.channel_integrations WHERE channel='whatsapp') AND organization_id=$1 AND status='delivery_unknown'),'retry_messages',(SELECT count(*) FROM platform.channel_inbox WHERE channel='whatsapp' AND organization_id=$1 AND status='pending' AND attempts>0),'configured', $2::boolean,'integration',(SELECT to_jsonb(i) FROM (SELECT id,channel,mode,phone_id,graph_version,active,sending_enabled,monetary_budget,window_seconds FROM platform.channel_integrations WHERE channel='whatsapp' AND organization_id=$1) i),'deliveries',coalesce((SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT id,status,version,attempts,provider_id,payload,created_at FROM platform.channel_outbox WHERE integration_id IN (SELECT id FROM platform.channel_integrations WHERE channel='whatsapp') AND organization_id=$1 ORDER BY (status='delivery_unknown') DESC,created_at DESC LIMIT 50)x),'[]'::jsonb))`, t.OrganizationID, len(s.Key) == 32).Scan(&b)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	return b, nil
}
func (s Service) Connect(ctx context.Context, t catalog.Tenant, in ConnectInput) (json.RawMessage, error) {
	if len(s.Key) != 32 {
		return nil, auth.Failure(503, "WHATSAPP_DISABLED", "Настройте отдельный серверный ключ WhatsApp")
	}
	if in.WindowSeconds == 0 {
		in.WindowSeconds = 86400
	}
	if in.RetentionDays == 0 {
		in.RetentionDays = 7
	}
	if in.WindowSeconds < 60 || in.WindowSeconds > 86400 || in.RetentionDays < 1 || in.RetentionDays > 30 {
		return nil, auth.Failure(422, "VALIDATION_ERROR", "Проверьте окно и срок хранения")
	}
	id := auth.UUID()
	allowed := map[string]bool{}
	for _, u := range in.Credentials.AllowedUsers {
		allowed[u] = true
	}
	c := Config{OrganizationID: t.OrganizationID, IntegrationID: id, AppID: in.AppID, BusinessAccountID: in.BusinessID, PhoneNumberID: in.PhoneID, GraphVersion: in.GraphVersion, AccessToken: in.Credentials.AccessToken, AppSecret: in.Credentials.AppSecret, VerifyToken: in.Credentials.VerifyToken, AllowedUsers: allowed, Window: time.Duration(in.WindowSeconds) * time.Second}
	if c.Validate() != nil || !stringsSynthetic(in.Credentials) {
		return nil, auth.Failure(422, "INVALID_MOCK_CONFIGURATION", "Только синтетическая конфигурация; секреты mock:...")
	}
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	raw, _ := json.Marshal(in.Credentials)
	cipher, e := seal(s.Key, t.OrganizationID, raw)
	if e != nil {
		return nil, auth.Unavailable()
	}
	h := sha256.Sum256(s.Proof())
	_, e = tx.Exec(ctx, `SELECT platform.channel_connect($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, t.OrganizationID, in.AppID, in.BusinessID, in.PhoneID, in.GraphVersion, cipher, h[:], in.WindowSeconds, in.RetentionDays)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	return s.Status(ctx, t)
}
func stringsSynthetic(c Credentials) bool {
	return len(c.AccessToken) >= 5 && c.AccessToken[:5] == "mock:" && len(c.AppSecret) >= 16 && len(c.VerifyToken) >= 16 && c.AppSecret[:5] == "mock:" && c.VerifyToken[:5] == "mock:"
}
func (s Service) Control(ctx context.Context, t catalog.Tenant, active, send bool) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT platform.channel_control($1,$2,$3)`, t.OrganizationID, active, send)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	return s.Status(ctx, t)
}
func (s Service) configuration(ctx context.Context, id string) (Config, error) {
	if len(s.Key) != 32 || !organizations.ValidID(id) {
		return Config{}, channel.ErrRejected
	}
	var raw []byte
	e := s.Pool.QueryRow(ctx, `SELECT platform.channel_credentials($1,$2)`, id, s.Proof()).Scan(&raw)
	if e != nil {
		return Config{}, auth.Unavailable()
	}
	if len(raw) == 0 || string(raw) == "null" {
		return Config{}, channel.ErrRejected
	}
	var row struct {
		ID           string `json:"id"`
		Organization string `json:"organization_id"`
		App          string `json:"app_id"`
		Business     string `json:"business_id"`
		Phone        string `json:"phone_id"`
		Version      string `json:"graph_version"`
		Cipher       string `json:"cipher"`
		Seconds      int    `json:"window_seconds"`
	}
	if json.Unmarshal(raw, &row) != nil {
		return Config{}, channel.ErrRejected
	}
	b, e := hex.DecodeString(row.Cipher)
	if e != nil {
		return Config{}, channel.ErrRejected
	}
	b, e = open(s.Key, row.Organization, b)
	if e != nil {
		return Config{}, channel.ErrRejected
	}
	var cred Credentials
	if json.Unmarshal(b, &cred) != nil {
		return Config{}, channel.ErrRejected
	}
	allowed := map[string]bool{}
	for _, u := range cred.AllowedUsers {
		allowed[u] = true
	}
	c := Config{OrganizationID: row.Organization, IntegrationID: id, AppID: row.App, BusinessAccountID: row.Business, PhoneNumberID: row.Phone, GraphVersion: row.Version, AccessToken: cred.AccessToken, AppSecret: cred.AppSecret, VerifyToken: cred.VerifyToken, AllowedUsers: allowed, Window: time.Duration(row.Seconds) * time.Second}
	return c, c.Validate()
}
func (s Service) Challenge(ctx context.Context, id string, query map[string][]string) (string, error) {
	c, e := s.configuration(ctx, id)
	if e != nil {
		return "", e
	}
	return c.Challenge(query)
}
func (s Service) Ingress(ctx context.Context, id, signature string, raw []byte) (int, error) {
	c, e := s.configuration(ctx, id)
	if e != nil {
		return 0, e
	}
	events, e := (Adapter{Config: c}).Normalize(ctx, channel.Envelope{IntegrationID: id, Header: map[string]string{"X-Hub-Signature-256": signature}, Body: raw, ReceivedAt: time.Now().UTC()})
	if e != nil {
		return 0, channel.ErrRejected
	}
	for i := range events {
		events[i].CorrelationID = auth.UUID()
	}
	b, _ := json.Marshal(events)
	var n int
	e = s.Pool.QueryRow(ctx, `SELECT platform.channel_ingress($1,$2,$3)`, id, s.Proof(), b).Scan(&n)
	if e != nil {
		return 0, tenancy.Error(e)
	}
	return n, nil
}
func (s Service) Resolve(ctx context.Context, t catalog.Tenant, id string, version int64, state string) error {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT platform.channel_resolve($1,$2,$3,$4)`, t.OrganizationID, id, version, state)
	if e != nil {
		var pe interface{ SQLState() string }
		if errors.As(e, &pe) && pe.SQLState() == "P0002" {
			return auth.Failure(404, "NOT_FOUND", "Ресурс не найден")
		}
		if errors.As(e, &pe) && pe.SQLState() == "40001" {
			return auth.Failure(409, "VERSION_CONFLICT", "Состояние доставки изменилось")
		}
		return tenancy.Error(e)
	}
	return tx.Commit(ctx)
}

// Inject is a protected synthetic fixture operation, not a public bypass of HMAC.
func (s Service) Inject(ctx context.Context, t catalog.Tenant, user, event, text, action string) (int, error) {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	var owner bool
	var id string
	if e = tx.QueryRow(ctx, `SELECT platform.has_membership($1,true)`, t.OrganizationID).Scan(&owner); e != nil || !owner {
		return 0, auth.Failure(403, "FORBIDDEN", "Только owner подаёт синтетические события")
	}
	if e = tx.QueryRow(ctx, `SELECT id FROM platform.channel_integrations WHERE channel='whatsapp' AND organization_id=$1 AND active`, t.OrganizationID).Scan(&id); e != nil {
		return 0, tenancy.Error(e)
	}
	_ = tx.Rollback(ctx)
	c, e := s.configuration(ctx, id)
	if e != nil {
		return 0, auth.Unavailable()
	}
	message := map[string]any{"from": user, "id": event, "timestamp": strconv.FormatInt(time.Now().Unix(), 10), "type": "text", "text": map[string]string{"body": text}}
	if action != "" {
		message["type"] = "interactive"
		message["interactive"] = map[string]any{"type": "button_reply", "button_reply": map[string]string{"id": action}}
	}
	raw, _ := json.Marshal(map[string]any{"object": "whatsapp_business_account", "entry": []any{map[string]any{"id": c.BusinessAccountID, "changes": []any{map[string]any{"field": "messages", "value": map[string]any{"metadata": map[string]string{"phone_number_id": c.PhoneNumberID}, "messages": []any{message}}}}}}})
	return s.Ingress(ctx, id, Signature(c.AppSecret, raw), raw)
}
