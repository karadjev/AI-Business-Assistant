package channel

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AdmitBookingNotification runs after normal channel admission, on its pinned connection.
func AdmitBookingNotification(ctx context.Context, conn *pgxpool.Conn, capability string, transport Type) (bool, *int64, error) {
	var allowed bool
	var key *int64
	err := conn.QueryRow(ctx, "SELECT allowed,lock_key FROM platform.booking_notification_admit($1,$2)", capability, string(transport)).Scan(&allowed, &key)
	return allowed, key, err
}

// UnlockBookingNotification reports whether the pinned session is safe to reuse.
func UnlockBookingNotification(ctx context.Context, conn *pgxpool.Conn, key *int64) bool {
	if key == nil {
		return true
	}
	var unlocked bool
	return conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", *key).Scan(&unlocked) == nil && unlocked
}
