package instagram

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
)

const MaxPayload = 65536

// Match the shared channel repository's numeric identity constraints. A signed
// but incompatible event must be rejected individually before atomic enqueue.
const maxExternalIdentityDigits = 20

var digits = regexp.MustCompile(`^[0-9]{1,32}$`)
var ErrRejected = errors.New("INSTAGRAM_REQUEST_REJECTED")

func VerifySignature(secret, signature string, raw []byte) bool {
	if len(secret) < 16 || len(secret) > 256 || len(raw) == 0 || len(raw) > MaxPayload || len(signature) != 71 || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(raw)
	return hmac.Equal(got, h.Sum(nil))
}

func Challenge(token string, q url.Values) (string, error) {
	if len(token) < 16 || len(token) > 256 || len(q["hub.mode"]) != 1 || len(q["hub.verify_token"]) != 1 || len(q["hub.challenge"]) != 1 || q.Get("hub.mode") != "subscribe" || subtle.ConstantTimeCompare([]byte(token), []byte(q.Get("hub.verify_token"))) != 1 || !digits.MatchString(q.Get("hub.challenge")) {
		return "", ErrRejected
	}
	return q.Get("hub.challenge"), nil
}

// Binding comes from a server-owned unique account mapping. No tenant fields
// supplied by Meta are used as authority. No media URLs are fetched here.
type Binding struct {
	OrganizationID string `json:"organization_id"`
	IntegrationID  string `json:"integration_id"`
	AccountID      string `json:"account_id"`
}
type Batch struct {
	Messages          []channel.Message
	Ignored, Rejected int
}
type event struct {
	Sender struct {
		ID string `json:"id"`
	} `json:"sender"`
	Recipient struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Timestamp int64 `json:"timestamp"`
	Message   *struct {
		ID          string `json:"mid"`
		Text        string `json:"text"`
		Echo        bool   `json:"is_echo"`
		Deleted     bool   `json:"is_deleted"`
		Unsupported bool   `json:"is_unsupported"`
		Reply       struct {
			ID string `json:"mid"`
		} `json:"reply_to"`
		Attachments []struct {
			Type string `json:"type"`
		} `json:"attachments"`
	} `json:"message"`
}

// Normalize requires prior HMAC verification. It bounds replay age; durable
// uniqueness on integration+mid is additionally required before acknowledgement.
// A malformed individual event does not suppress valid siblings.
func Normalize(b Binding, raw []byte, received time.Time) (Batch, error) {
	var out Batch
	if b.OrganizationID == "" || b.IntegrationID == "" || !digits.MatchString(b.AccountID) || len(b.AccountID) > maxExternalIdentityDigits || received.IsZero() || len(raw) == 0 || len(raw) > MaxPayload || !utf8.Valid(raw) {
		return out, ErrRejected
	}
	var w struct {
		Object  string `json:"object"`
		Entries []struct {
			ID        string            `json:"id"`
			Messaging []json.RawMessage `json:"messaging"`
		} `json:"entry"`
	}
	if json.Unmarshal(raw, &w) != nil || w.Object != "instagram" || len(w.Entries) == 0 || len(w.Entries) > 10 {
		return out, ErrRejected
	}
	count := 0
	for _, entry := range w.Entries {
		count += len(entry.Messaging)
		if count > 100 {
			return Batch{}, ErrRejected
		}
		if entry.ID != b.AccountID {
			return Batch{}, ErrRejected
		}
	}
	seen := map[string]bool{}
	for _, entry := range w.Entries {
		for _, rawEvent := range entry.Messaging {
			var e event
			if json.Unmarshal(rawEvent, &e) != nil {
				out.Rejected++
				continue
			}
			if e.Message == nil || e.Message.Echo {
				out.Ignored++
				continue
			}
			m := e.Message
			ts := time.UnixMilli(e.Timestamp).UTC()
			if e.Recipient.ID != b.AccountID || !digits.MatchString(e.Sender.ID) || len(e.Sender.ID) > maxExternalIdentityDigits || e.Sender.ID == b.AccountID || !identifier(m.ID) || (m.Reply.ID != "" && !identifier(m.Reply.ID)) || ts.Before(received.Add(-24*time.Hour)) || ts.After(received.Add(5*time.Minute)) || len(m.Attachments) > 10 || !utf8.ValidString(m.Text) || len([]rune(m.Text)) > 2000 || strings.ContainsRune(m.Text, 0) {
				out.Rejected++
				continue
			}
			if seen[m.ID] {
				out.Ignored++
				continue
			}
			seen[m.ID] = true
			n := channel.Message{OrganizationID: b.OrganizationID, IntegrationID: b.IntegrationID, Channel: channel.Instagram, ExternalUserID: e.Sender.ID, ExternalChatID: e.Sender.ID, ExternalMessageID: m.ID, Direction: "inbound", Kind: channel.Text, Text: m.Text, ReplyTo: m.Reply.ID, ProviderTimestamp: ts, ReceivedAt: received}
			// The common conversation service requires a UUID idempotency key.
			h := sha256.Sum256([]byte(b.OrganizationID + "/" + b.IntegrationID + "/instagram/" + m.ID))
			h[6] = h[6]&15 | 64
			h[8] = h[8]&63 | 128
			x := hex.EncodeToString(h[:16])
			n.CorrelationID = x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:]
			for _, a := range m.Attachments {
				kind := a.Type
				switch kind {
				case "image", "video", "audio", "file", "story_mention", "share":
				default:
					kind = "unsupported"
				}
				n.Attachments = append(n.Attachments, channel.Attachment{Type: kind})
			}
			if len(n.Attachments) > 0 || m.Unsupported || m.Deleted || strings.TrimSpace(m.Text) == "" {
				n.Kind = channel.Unsupported
			}
			out.Messages = append(out.Messages, n)
		}
	}
	return out, nil
}

func identifier(s string) bool {
	return len(s) > 0 && len(s) <= 256 && utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\r\n")
}
