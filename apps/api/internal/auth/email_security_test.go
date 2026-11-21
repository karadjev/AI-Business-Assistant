package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func boundaryEmail(size int) string {
	return strings.Repeat("a", 64) + "@" + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", size-193)
}

func requireEmailError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Status != status || failure.Code != code {
		t.Fatal("unexpected email rejection status/code")
	}
}

func TestEmailValidation(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"normal", "user@example.test", "user@example.test"},
		{"normalization", "  User+TAG@Example.TEST \t", "user+tag@example.test"},
		{"unicode_outer_space", "\u2003A@EXAMPLE.TEST\u2003", "a@example.test"},
		{"boundary_253", boundaryEmail(253), boundaryEmail(253)},
		{"boundary_254", boundaryEmail(254), boundaryEmail(254)},
		{"empty", "", ""},
		{"whitespace", " \t\r\n", ""},
		{"missing_at", "not-email", ""},
		{"missing_domain_dot", "a@example", ""},
		{"display_name", "Name <a@example.test>", ""},
		{"newline", "a\r\n@example.test", ""},
		{"nul", "a\x00@example.test", ""},
		{"unicode_local", "пользователь@example.test", ""},
		{"unicode_domain", "a@пример.test", ""},
		{"invalid_utf8", "a\xff@example.test", ""},
		{"boundary_255", boundaryEmail(255), ""},
		{"long_ascii", strings.Repeat("a", 16384) + "@example.test", ""},
		{"long_punctuation", strings.Repeat("(\"", 8192) + "@example.test", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() != nil {
					t.Fatal("email validation panicked")
				}
			}()
			got, err := Email(tc.input)
			if tc.want == "" {
				requireEmailError(t, err, 422, "VALIDATION_ERROR")
				if got != "" {
					t.Fatal("rejected email returned a value")
				}
			} else if err != nil || got != tc.want {
				t.Fatal("valid email normalization failed")
			}
		})
	}
}

// This test is also run alone with coverage to verify the parser block is not
// executed for admission failures. It never submits an exploit to a live service.
func TestEmailAdmissionRejectsBeforeParser(t *testing.T) {
	for _, input := range []string{"", " \t", boundaryEmail(255), strings.Repeat("(\"", 8192), "a@пример.test", "a\xff@example.test"} {
		got, err := Email(input)
		requireEmailError(t, err, 422, "VALIDATION_ERROR")
		if got != "" {
			t.Fatal("admission failure returned an email")
		}
	}
}

func TestEmailRepeatedConcurrentRejection(t *testing.T) {
	input := strings.Repeat("a", 4096) + "@example.test"
	var wg sync.WaitGroup
	failed := make(chan struct{}, 8)
	for range 8 {
		wg.Go(func() {
			defer func() {
				if recover() != nil {
					failed <- struct{}{}
				}
			}()
			for range 128 {
				got, err := Email(input)
				var failure *Error
				if got != "" || !errors.As(err, &failure) || failure.Status != 422 || failure.Code != "VALIDATION_ERROR" {
					failed <- struct{}{}
					return
				}
			}
		})
	}
	wg.Wait()
	close(failed)
	if len(failed) != 0 {
		t.Fatal("repeated email rejection changed or panicked")
	}
}

func TestEmailServiceAdmissionBeforeDatabase(t *testing.T) {
	// A missing pool makes any accidental database access fail this regression.
	s := &Service{}
	ctx := context.Background()
	input := boundaryEmail(255)
	for range 3 {
		requireEmailError(t, s.Register(ctx, input, "Synthetic-Password-123!", "Synthetic"), 422, "VALIDATION_ERROR")
		requireEmailError(t, s.Resend(ctx, input), 422, "VALIDATION_ERROR")
		user, token, csrf, err := s.Login(ctx, input, "Synthetic-Password-123!", "")
		requireEmailError(t, err, 401, "INVALID_CREDENTIALS")
		if user.ID != "" || token != "" || csrf != "" {
			t.Fatal("invalid login returned identity/session material")
		}
	}
}
