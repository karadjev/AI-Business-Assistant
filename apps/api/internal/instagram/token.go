package instagram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OfflineOAuth implements Instagram Login wire requests exclusively against an
// injected fixture. It accepts synthetic secrets/tokens only and is not runtime wiring.
type OfflineOAuth struct {
	AppID, RedirectURI string
	AppSecret          string `json:"-"`
	RoundTrip          func(*http.Request) (*http.Response, error)
}
type Token struct {
	AccessToken         string `json:"-"`
	AccountID           string
	Permissions         []string
	IssuedAt, ExpiresAt time.Time
}

func (o OfflineOAuth) call(ctx context.Context, method, endpoint string, values url.Values, out any) error {
	if o.RoundTrip == nil || !digits.MatchString(o.AppID) || !strings.HasPrefix(o.AppSecret, "mock:") || strings.ContainsAny(o.AppSecret, "\r\n\x00") {
		return ErrRejected
	}
	var body io.Reader
	if method == http.MethodGet {
		endpoint += "?" + values.Encode()
	} else {
		body = strings.NewReader(values.Encode())
	}
	r, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return ErrRejected
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := o.RoundTrip(r)
	if err != nil || response == nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return ErrRejected
	}
	if response.Body == nil {
		return ErrRejected
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxPayload+1))
	if err != nil || len(raw) > MaxPayload || response.StatusCode != 200 || json.Unmarshal(raw, out) != nil {
		return ErrRejected
	}
	return nil
}

// Exchange is called only after Callback successfully consumes state. Permissions
// are verified against the exchange result; subscription verification is separate.
func (o OfflineOAuth) Exchange(ctx context.Context, code string, now time.Time) (Token, error) {
	b := StateBinding{"tenant", "user", "session", o.RedirectURI}
	if !validBinding(b) || len(code) == 0 || len(code) > 4096 || strings.ContainsAny(code, "\r\n\x00") || now.IsZero() {
		return Token{}, ErrRejected
	}
	var short struct {
		Data []struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"user_id"`
			Permissions string `json:"permissions"`
		} `json:"data"`
	}
	if o.call(ctx, http.MethodPost, "https://api.instagram.com/oauth/access_token", url.Values{"client_id": {o.AppID}, "client_secret": {o.AppSecret}, "grant_type": {"authorization_code"}, "redirect_uri": {o.RedirectURI}, "code": {code}}, &short) != nil || len(short.Data) != 1 {
		return Token{}, ErrRejected
	}
	s := short.Data[0]
	granted := map[string]bool{}
	for _, p := range strings.Split(s.Permissions, ",") {
		granted[strings.TrimSpace(p)] = true
	}
	if !digits.MatchString(s.AccountID) || !syntheticToken(s.AccessToken) || !granted["instagram_business_basic"] || !granted["instagram_business_manage_messages"] {
		return Token{}, ErrRejected
	}
	return o.longToken(ctx, "https://graph.instagram.com/access_token", url.Values{"grant_type": {"ig_exchange_token"}, "client_secret": {o.AppSecret}, "access_token": {s.AccessToken}}, Token{AccountID: s.AccountID, Permissions: []string{"instagram_business_basic", "instagram_business_manage_messages"}}, now)
}

func (o OfflineOAuth) Refresh(ctx context.Context, token Token, now time.Time) (Token, error) {
	if now.IsZero() || token.IssuedAt.IsZero() || now.Before(token.IssuedAt.Add(24*time.Hour)) || !token.ExpiresAt.After(now) || !syntheticToken(token.AccessToken) || !digits.MatchString(token.AccountID) {
		return Token{}, ErrRejected
	}
	return o.longToken(ctx, "https://graph.instagram.com/refresh_access_token", url.Values{"grant_type": {"ig_refresh_token"}, "access_token": {token.AccessToken}}, token, now)
}

func (o OfflineOAuth) longToken(ctx context.Context, endpoint string, values url.Values, token Token, now time.Time) (Token, error) {
	var result struct {
		AccessToken string `json:"access_token"`
		Expires     int64  `json:"expires_in"`
	}
	if o.call(ctx, http.MethodGet, endpoint, values, &result) != nil || !syntheticToken(result.AccessToken) || result.Expires <= 0 || result.Expires > 60*24*60*60 {
		return Token{}, ErrRejected
	}
	token.AccessToken = result.AccessToken
	token.IssuedAt = now
	token.ExpiresAt = now.Add(time.Duration(result.Expires) * time.Second)
	return token, nil
}
func syntheticToken(s string) bool {
	return strings.HasPrefix(s, "mock:") && len(s) > 5 && len(s) <= 4096 && !strings.ContainsAny(s, "\r\n\x00")
}
