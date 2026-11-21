package instagram

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
)

func TestCredentialKeyRotationFailsClosedAndSecretsAreNotSerialized(t *testing.T) {
	oldKey, newKey := bytes.Repeat([]byte{31}, 32), bytes.Repeat([]byte{32}, 32)
	oldBox := CredentialBox{Key: oldKey}
	sealed, err := oldBox.Seal(binding(), []byte("mock:synthetic-secret-token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (CredentialBox{Key: newKey}).Open(binding(), sealed); err == nil {
		t.Fatal("ciphertext from a previous key accepted")
	}
	if _, err = oldBox.Open(binding(), sealed); err != nil {
		t.Fatal("old key recovery unexpectedly lost", err)
	}
	for _, value := range []any{CredentialBox{Key: oldKey}, Token{AccessToken: "mock:synthetic-secret-token"}, SendConfig{AccessToken: "mock:synthetic-secret-token"}, config.Instagram{Enabled: true, Key: oldKey, AppSecret: "mock:synthetic-secret-token", VerifyToken: "mock:synthetic-secret-token"}} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("mock:synthetic-secret-token")) || bytes.Contains(raw, []byte("access_token")) || bytes.Contains(raw, []byte("AppSecret")) || bytes.Contains(raw, []byte("VerifyToken")) || bytes.Contains(raw, []byte("Key")) {
			t.Fatal("credential serialized")
		}
	}
}
