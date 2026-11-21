package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
)

func TestMockProvenanceCannotClaimOfficial(t *testing.T) {
	d := ProviderDocument{ID: "synthetic", SourceType: "OFFICIAL_PROVIDER", SourceIdentity: "meta", VerificationMethod: "official", Payload: json.RawMessage(`{"synthetic":true}`)}
	for _, kind := range []string{"pricing", "template"} {
		var got ProviderDocument
		var e error
		if kind == "pricing" {
			got, e = (MockPricingSource{Document: d}).Pricing(context.Background(), Config{})
		} else {
			got, e = (MockTemplateSource{Document: d}).TemplateStatus(context.Background(), Config{})
		}
		if e != nil || got.SourceType != "SYNTHETIC_TEST" || got.SourceIdentity != "synthetic:offline-fixture" || got.VerificationMethod != "OFFLINE_DIGEST" || got.Digest != sha256.Sum256(d.Payload) {
			t.Fatal("mock manufactured official attestation")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := (MockPricingSource{Document: d}).Pricing(ctx, Config{}); e == nil {
		t.Fatal("cancelled source accepted")
	}
}
func TestReconciliationRequiresExplicitConfirmation(t *testing.T) {
	_, e := (Service{}).ReconcileEvidence(context.Background(), catalog.Tenant{}, "invalid", ReconciliationInput{EvidenceAt: time.Now()})
	if e == nil {
		t.Fatal("unconfirmed reconciliation accepted")
	}
}
