//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

type whatsappCrashAfterReceipt struct{ wire whatsapp.Transport }

func (t whatsappCrashAfterReceipt) Send(ctx context.Context, c whatsapp.Config, m channel.Message) (channel.SendResult, error) {
	r, e := t.wire.Send(ctx, c, m)
	if e == nil && r.State == channel.Sent {
		os.Exit(87)
	}
	return r, e
}
func TestWhatsAppCrashHelper(t *testing.T) {
	if os.Getenv("ABA_WA_CRASH") != "after_receipt" {
		t.Skip("subprocess helper only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, e := db.Open(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"), 2, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	target, e := url.Parse(os.Getenv("ABA_WA_WIRE"))
	if e != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" {
		t.Fatal("non-loopback fixture")
	}
	tr := &http.Transport{Proxy: nil}
	defer tr.CloseIdleConnections()
	wire := whatsapp.HTTPMock{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "whatsapp.mock.invalid" {
			return nil, errors.New("unexpected destination")
		}
		copy := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = target.Scheme
		u.Host = target.Host
		copy.URL = &u
		return tr.RoundTrip(copy)
	})}
	s := whatsapp.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{61}, 32), Transport: whatsappCrashAfterReceipt{wire: wire}}
	if _, e = s.SendOne(ctx); e != nil {
		t.Fatal(e)
	}
	t.Fatal("crash hook not reached")
}
