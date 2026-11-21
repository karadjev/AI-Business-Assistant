package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestMockAndEncryptedSecrets(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	c, e := encrypt(key, "mock:123", "org-a")
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(c, []byte("mock:123")) {
		t.Fatal("plaintext persisted")
	}
	if _, e = decrypt(key, c, "org-b"); e == nil {
		t.Fatal("AAD not bound")
	}
	plain, e := decrypt(key, c, "org-a")
	if e != nil || plain != "mock:123" {
		t.Fatal("roundtrip")
	}
	c[len(c)-1] ^= 1
	if _, e = decrypt(key, c, "org-a"); e == nil {
		t.Fatal("tamper accepted")
	}
	if _, e = (Mock{}).GetMe(context.Background(), "real-token"); e == nil {
		t.Fatal("real credential accepted")
	}
	if _, e = (Mock{}).Send(context.Background(), "mock:123", 1, json.RawMessage(`{"text":"synthetic"}`)); e != nil {
		t.Fatal(e)
	}
}
func TestPrincipalPrivateOnly(t *testing.T) {
	good := Update{ID: 1, Message: &Message{From: User{ID: 42}, Chat: Chat{42, "private"}, Text: "TEST6"}}
	if u, c, e := good.Principal(); e != nil || u != 42 || c != 42 {
		t.Fatal("valid principal")
	}
	bad := good
	copy := *good.Message
	bad.Message = &copy
	bad.Message.Chat.Type = "group"
	if _, _, e := bad.Principal(); e == nil {
		t.Fatal("group accepted")
	}
	bad.Message.Chat.Type = "private"
	bad.Message.Chat.ID = 41
	if _, _, e := bad.Principal(); e == nil {
		t.Fatal("cross-user chat accepted")
	}
	good.Callback = &Callback{}
	if _, _, e := good.Principal(); e == nil {
		t.Fatal("ambiguous update accepted")
	}
}
