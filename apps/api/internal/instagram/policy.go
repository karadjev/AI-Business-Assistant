// Package instagram contains the synthetic Instagram Login runtime and official
// API wire contracts. External requests and outbound delivery remain disabled.
package instagram

import (
	"errors"
	"time"
)

// Policy must be loaded from trusted connection/principal state, never webhook JSON.
// Exceptions (Human Agent, ads, marketing) require separate evidence and are disabled.
type Policy struct {
	Active, SendingEnabled, PermissionsVerified, WebhookVerified bool
	Blocked, DeliveryUnknown                                     bool
	TokenExpiresAt, LastInbound                                  time.Time
}

func (p Policy) Check(now time.Time) error {
	switch {
	case now.IsZero(), !p.Active, !p.SendingEnabled:
		return errors.New("INSTAGRAM_SENDING_DISABLED")
	case !p.PermissionsVerified:
		return errors.New("INSTAGRAM_LIMITED_PERMISSIONS")
	case !p.WebhookVerified:
		return errors.New("INSTAGRAM_WEBHOOK_UNVERIFIED")
	case !p.TokenExpiresAt.After(now):
		return errors.New("INSTAGRAM_REAUTH_REQUIRED")
	case p.Blocked:
		return errors.New("INSTAGRAM_RECIPIENT_UNAVAILABLE")
	case p.DeliveryUnknown:
		return errors.New("DELIVERY_UNKNOWN")
	case p.LastInbound.IsZero(), p.LastInbound.After(now), !now.Before(p.LastInbound.Add(24 * time.Hour)):
		return errors.New("INSTAGRAM_MESSAGING_WINDOW_CLOSED")
	}
	return nil
}

// RetryDelay is for confirmed retryable rejections only. Timeouts/5xx/unknown
// delivery must never enter this path. A durable worker must enforce attempts.
func RetryDelay(attempt int, retryAfter time.Duration) (time.Duration, bool) {
	if attempt < 1 || attempt >= 5 || retryAfter < 0 || retryAfter > time.Hour {
		return 0, false
	}
	return max(time.Second*time.Duration(1<<attempt), retryAfter), true
}
