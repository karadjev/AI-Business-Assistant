package auth

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

func TestIdentityEmailKeepsDirectLinksInBothFormats(t *testing.T) {
	for _, tc := range []struct{ kind, path, action string }{
		{"verification", "/verify-email", "Подтвердить email"},
		{"password_reset", "/reset-password", "Задать новый пароль"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			link := "https://example.test" + tc.path + "#token=" + strings.Repeat("a", 43)
			recipient := "owner@example.test"
			raw, err := renderIdentityEmail(mail.Address{Address: "no-reply@example.test"}, recipient, emailPayload{Link: link, Kind: tc.kind})
			if err != nil {
				t.Fatal(err)
			}
			message, err := mail.ReadMessage(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			format, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
			if err != nil || format != "multipart/alternative" || message.Header.Get("To") != recipient {
				t.Fatal("unexpected identity email headers")
			}
			reader := multipart.NewReader(message.Body, params["boundary"])
			for _, format := range []string{"text/plain", "text/html"} {
				part, err := reader.NextPart()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(part)
				if err != nil || part.Header.Get("Content-Type") != format+"; charset=UTF-8" || !bytes.Contains(data, []byte(link)) {
					t.Fatal("identity link changed or missing from " + format)
				}
				if format == "text/html" && !bytes.Contains(data, []byte(tc.action)) {
					t.Fatal("identity action missing from HTML")
				}
			}
			if _, err := reader.NextPart(); err != io.EOF {
				t.Fatal("unexpected additional part")
			}
		})
	}
}
