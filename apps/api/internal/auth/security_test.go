package auth

import (
	"bytes"
	"testing"
)

func TestSecurity(t *testing.T) {
	for _, v := range []string{"a", "12345678901"} {
		if PasswordValid(v) {
			t.Fatal("weak password accepted")
		}
	}
	if !PasswordValid("a-good-password-123") {
		t.Fatal("valid password rejected")
	}
	hash := PasswordHash("a-good-password-123")
	if !PasswordMatches("a-good-password-123", hash) || PasswordMatches("wrong-password-123", hash) || PasswordMatches("whatever", hash+"bad") {
		t.Fatal("hash check failed")
	}
	email, e := Email("  A@EXAMPLE.TEST ")
	if e != nil || email != "a@example.test" {
		t.Fatal("normalization")
	}
	for _, bad := range []string{"not-email", "Name <a@example.test>", "a\r\n@example.test"} {
		if _, e = Email(bad); e == nil {
			t.Fatal("invalid email")
		}
	}
	if len(Token()) != 43 || len(Hash("test")) != 32 {
		t.Fatal("token entropy/size")
	}
}
func TestEncryptedOutbox(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	plain := []byte("sensitive-verification-token")
	encrypted, e := encrypt(key, plain)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(encrypted, plain) {
		t.Fatal("plaintext stored")
	}
	decoded, e := decrypt(key, encrypted)
	if e != nil || !bytes.Equal(decoded, plain) {
		t.Fatal("encryption roundtrip")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, e = decrypt(key, encrypted); e == nil {
		t.Fatal("tampered payload accepted")
	}
}
