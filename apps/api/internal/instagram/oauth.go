package instagram

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"time"
)

const Permissions = "instagram_business_basic,instagram_business_manage_messages"

// StateStore must atomically consume an unexpired, unused record bound to all
// four fields in the existing tenant database. Never implement it as a cookie alone.
type StateStore interface {
	Create(context.Context, [32]byte, StateBinding, time.Time) error
	Consume(context.Context, [32]byte, StateBinding, time.Time) error
}
type StateBinding struct{ OrganizationID, UserID, SessionID, RedirectURI string }
type OAuth struct {
	AppID, RedirectURI string
	States             StateStore
}

func (o OAuth) Start(ctx context.Context, b StateBinding, now time.Time) (string, error) {
	if o.States == nil || !digits.MatchString(o.AppID) || !validBinding(b) || b.RedirectURI != o.RedirectURI || now.IsZero() {
		return "", ErrRejected
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", ErrRejected
	}
	state := base64.RawURLEncoding.EncodeToString(random)
	if err := o.States.Create(ctx, sha256.Sum256([]byte(state)), b, now.Add(10*time.Minute)); err != nil {
		return "", ErrRejected
	}
	q := url.Values{"client_id": {o.AppID}, "redirect_uri": {o.RedirectURI}, "response_type": {"code"}, "scope": {Permissions}, "state": {state}, "enable_fb_login": {"false"}}
	return "https://www.instagram.com/oauth/authorize?" + q.Encode(), nil
}

// Callback consumes state even on cancellation, before any token exchange. Caller
// must derive binding from the authenticated session and sanitize callback logs.
func (o OAuth) Callback(ctx context.Context, b StateBinding, q url.Values, now time.Time) (string, error) {
	if o.States == nil || !validBinding(b) || b.RedirectURI != o.RedirectURI || now.IsZero() || len(q["state"]) != 1 {
		return "", ErrRejected
	}
	state := q.Get("state")
	decoded, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != state {
		return "", ErrRejected
	}
	if o.States.Consume(ctx, sha256.Sum256([]byte(state)), b, now) != nil {
		return "", ErrRejected
	}
	code := q.Get("code")
	if len(q["error"]) > 0 || len(q["code"]) != 1 || len(code) == 0 || len(code) > 4096 || strings.ContainsAny(code, "\x00\r\n") {
		return "", ErrRejected
	}
	return q.Get("code"), nil
}

func validBinding(b StateBinding) bool {
	u, err := url.Parse(b.RedirectURI)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == "" && u.RawQuery == "" && b.OrganizationID != "" && b.UserID != "" && b.SessionID != ""
}

// CredentialBox uses AES-256-GCM and binds ciphertext to tenant, integration,
// account and login model. A token from a different binding cannot be opened.
type CredentialBox struct {
	Key []byte `json:"-"`
}

func credentialAAD(b Binding) ([]byte, error) {
	if b.OrganizationID == "" || b.IntegrationID == "" || !digits.MatchString(b.AccountID) || strings.ContainsAny(b.OrganizationID+b.IntegrationID, "/\x00") {
		return nil, ErrRejected
	}
	return []byte(b.OrganizationID + "/" + b.IntegrationID + "/" + b.AccountID + "/instagram-login/v1"), nil
}
func (c CredentialBox) aead() (cipher.AEAD, error) {
	if len(c.Key) != 32 {
		return nil, ErrRejected
	}
	b, err := aes.NewCipher(c.Key)
	if err != nil {
		return nil, ErrRejected
	}
	return cipher.NewGCM(b)
}
func (c CredentialBox) Seal(b Binding, token []byte) ([]byte, error) {
	aad, err := credentialAAD(b)
	if err != nil || len(token) == 0 || len(token) > 4096 {
		return nil, ErrRejected
	}
	g, err := c.aead()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, ErrRejected
	}
	return g.Seal(nonce, nonce, token, aad), nil
}
func (c CredentialBox) Open(b Binding, ciphertext []byte) ([]byte, error) {
	aad, err := credentialAAD(b)
	if err != nil {
		return nil, err
	}
	g, err := c.aead()
	if err != nil || len(ciphertext) < g.NonceSize()+g.Overhead() || len(ciphertext) > 4096+g.NonceSize()+g.Overhead() {
		return nil, ErrRejected
	}
	result, err := g.Open(nil, ciphertext[:g.NonceSize()], ciphertext[g.NonceSize():], aad)
	if err != nil {
		return nil, ErrRejected
	}
	return result, nil
}
