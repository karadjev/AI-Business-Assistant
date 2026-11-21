package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/argon2"
	"net/mail"
	"strings"
	"unicode/utf8"
)

func Token() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func Hash(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }
func UUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func Email(s string) (string, error) {
	s = strings.TrimSpace(s)
	// Bound untrusted input before normalization and the RFC 5322 parser.
	if len(s) == 0 || len(s) > 254 {
		return "", Failure(422, "VALIDATION_ERROR", "Введите корректный email")
	}
	for _, c := range s {
		if c > 127 {
			return "", Failure(422, "VALIDATION_ERROR", "Используйте email в ASCII формате")
		}
	}
	s = strings.ToLower(s)
	v, e := mail.ParseAddress(s)
	if e != nil || v.Address != s || !strings.Contains(strings.Split(s, "@")[len(strings.Split(s, "@"))-1], ".") {
		return "", Failure(422, "VALIDATION_ERROR", "Введите корректный email")
	}
	return s, nil
}
func PasswordValid(s string) bool {
	return utf8.RuneCountInString(s) >= 12 && utf8.RuneCountInString(s) <= 128 && len(s) <= 512 && !strings.ContainsRune(s, 0)
}
func PasswordHash(s string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	hash := argon2.IDKey([]byte(s), salt, 2, 64*1024, 1, 32)
	return "$argon2id$v=19$m=65536,t=2,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
}
func PasswordMatches(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=2,p=1" {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[4])
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if e != nil || err != nil || len(salt) != 16 || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 2, 64*1024, 1, 32)
	return subtle.ConstantTimeCompare(expected, actual) == 1
}
func encrypt(key, plain []byte) ([]byte, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return nil, e
	}
	return g.Seal(n, n, plain, []byte("email-verification-v1")), nil
}
func decrypt(key, data []byte) ([]byte, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(b)
	if e != nil || len(data) < g.NonceSize() {
		return nil, fmt.Errorf("invalid encrypted email")
	}
	return g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], []byte("email-verification-v1"))
}
