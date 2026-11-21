package instagram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOfflineOAuthExchangeAndRefresh(t *testing.T) {
	calls := 0
	o := OfflineOAuth{AppID: "100", AppSecret: "mock:synthetic-secret", RedirectURI: "https://avero.example/callback", RoundTrip: func(r *http.Request) (*http.Response, error) {
		calls++
		body := ""
		switch r.URL.Host + r.URL.Path {
		case "api.instagram.com/oauth/access_token":
			if r.Method != "POST" || r.ParseForm() != nil || r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("redirect_uri") != "https://avero.example/callback" {
				t.Fatal("code exchange contract")
			}
			body = `{"data":[{"access_token":"mock:short","user_id":"100","permissions":"instagram_business_basic,instagram_business_manage_messages"}]}`
		case "graph.instagram.com/access_token":
			if r.URL.Query().Get("grant_type") != "ig_exchange_token" {
				t.Fatal("long token grant")
			}
			body = `{"access_token":"mock:long","expires_in":5184000}`
		case "graph.instagram.com/refresh_access_token":
			if r.URL.Query().Get("grant_type") != "ig_refresh_token" {
				t.Fatal("refresh grant")
			}
			body = `{"access_token":"mock:refreshed","expires_in":5184000}`
		default:
			t.Fatal("wrong auth model/endpoint")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	}}
	token, err := o.Exchange(context.Background(), "synthetic-code", testNow)
	if err != nil || calls != 2 || token.AccountID != "100" || !token.ExpiresAt.Equal(testNow.Add(60*24*time.Hour)) {
		t.Fatal("exchange failed", err)
	}
	if _, err := o.Refresh(context.Background(), token, testNow.Add(time.Hour)); err == nil || calls != 2 {
		t.Fatal("premature refresh")
	}
	if _, err := o.Refresh(context.Background(), token, token.ExpiresAt); err == nil || calls != 2 {
		t.Fatal("expired refresh")
	}
	refreshed, err := o.Refresh(context.Background(), token, testNow.Add(24*time.Hour))
	if err != nil || refreshed.AccessToken != "mock:refreshed" || calls != 3 {
		t.Fatal("refresh failed")
	}
}

func TestOfflineOAuthRejectsPermissionsAndProviderErrors(t *testing.T) {
	for _, body := range []string{`{"data":[{"access_token":"mock:short","user_id":"100","permissions":"instagram_business_basic"}]}`, `{"data":[]}`, `{"error_type":"OAuthException","error_message":"synthetic-secret"}`, `{`} {
		calls := 0
		o := OfflineOAuth{AppID: "100", AppSecret: "mock:synthetic-secret", RedirectURI: "https://avero.example/callback", RoundTrip: func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		}}
		_, err := o.Exchange(context.Background(), "synthetic-code", testNow)
		if err == nil || calls != 1 || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("invalid exchange accepted or leaked")
		}
	}
}
