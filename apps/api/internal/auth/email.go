package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net"
	"net/smtp"
	"time"
)

type emailPayload struct {
	To   string
	Link string
	Kind string
}

func (s *Service) DeliverOne(ctx context.Context) (bool, error) {
	var id string
	var payload []byte
	e := s.Pool.QueryRow(ctx, `WITH candidate AS (SELECT id FROM platform.email_outbox WHERE status='pending' AND available_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE platform.email_outbox SET lease_until=now()+interval '30 seconds',attempts=attempts+1 WHERE id=(SELECT id FROM candidate) RETURNING id,payload`).Scan(&id, &payload)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, Unavailable()
	}
	plain, e := decrypt(s.Config.EmailKey, payload)
	var mail emailPayload
	if e == nil {
		e = json.Unmarshal(plain, &mail)
	}
	if e == nil {
		e = send(ctx, s.Config.SMTPAddr, mail)
	}
	if e != nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE platform.email_outbox SET lease_until=NULL,available_at=now()+interval '15 seconds',status=CASE WHEN attempts>=5 THEN 'failed' ELSE 'pending' END WHERE id=$1`, id)
		return true, errors.New("email delivery failed")
	}
	_, e = s.Pool.Exec(ctx, `UPDATE platform.email_outbox SET status='sent',payload='\x'::bytea,lease_until=NULL WHERE id=$1`, id)
	if e != nil {
		return true, Unavailable()
	}
	return true, nil
}
func send(ctx context.Context, addr string, msg emailPayload) error {
	conn, e := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", addr)
	if e != nil {
		return e
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	client, e := smtp.NewClient(conn, "mailpit")
	if e != nil {
		return e
	}
	defer client.Close()
	if e = client.Mail("no-reply@ai-business-assistant.test"); e != nil {
		return e
	}
	if e = client.Rcpt(msg.To); e != nil {
		return e
	}
	writer, e := client.Data()
	if e != nil {
		return e
	}
	subject, body := "Verify your email", "Confirm your email (link expires):\r\n"
	if msg.Kind == "password_reset" {
		subject, body = "Reset your password", "Reset your password using this one-time link (expires shortly):\r\n"
	}
	_, e = fmt.Fprintf(writer, "From: AI Business Assistant <no-reply@ai-business-assistant.test>\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s%s\r\n", msg.To, subject, body, msg.Link)
	if e != nil {
		return e
	}
	if e = writer.Close(); e != nil {
		return e
	}
	return client.Quit()
}
