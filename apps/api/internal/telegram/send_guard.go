package telegram

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"time"
)

// A pinned, idle DB connection holds the send admission lock, not a transaction.
// Disconnect/handoff use the same lock and cannot complete ahead of an admitted send.
type sendGuard struct {
	conn       *pgxpool.Conn
	key        int64
	bookingKey *int64
}

func (s Service) prepareSend(ctx context.Context, cap string) (*sendGuard, error) {
	conn, e := s.Pool.Acquire(ctx)
	if e != nil {
		return nil, e
	}
	var key *int64
	e = conn.QueryRow(ctx, `SELECT platform.telegram_prepare_send($1,$2,$3,$4)`, cap, s.WorkerProof(), s.Config.Mode, s.Config.AllowedIDs()).Scan(&key)
	if e != nil { // Cancellation may race lock acquisition: destroy, never pool this session.
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = conn.Hijack().Close(closeCtx)
		return nil, e
	}
	if key == nil {
		conn.Release()
		return nil, nil
	}
	guard := &sendGuard{conn: conn, key: *key}
	allowed, bookingKey, err := channel.AdmitBookingNotification(ctx, conn, cap, channel.Telegram)
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = conn.Hijack().Close(cleanup)
		return nil, err
	}
	guard.bookingKey = bookingKey
	if !allowed {
		guard.close(ctx)
		return nil, nil
	}
	return guard, nil
}
func (g *sendGuard) close(ctx context.Context) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if !channel.UnlockBookingNotification(cleanup, g.conn, g.bookingKey) {
		_ = g.conn.Hijack().Close(cleanup)
		return
	}
	var unlocked bool
	e := g.conn.QueryRow(cleanup, `SELECT pg_advisory_unlock($1)`, g.key).Scan(&unlocked)
	if e != nil || !unlocked {
		_ = g.conn.Hijack().Close(cleanup)
		return
	}
	g.conn.Release()
}
