package auth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/mail"
	"net/smtp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
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
		e = send(ctx, s.Config.SMTPAddr, s.Config.SMTP, mail)
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
func send(ctx context.Context, addr string, transport config.SMTP, msg emailPayload) error {
	recipient, e := Email(msg.To)
	if e != nil {
		return e
	}
	conn, e := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", addr)
	if e != nil {
		return e
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline := time.Now().Add(8 * time.Second)
	if until, ok := ctx.Deadline(); ok && until.Before(deadline) {
		deadline = until
	}
	if e = conn.SetDeadline(deadline); e != nil {
		return e
	}
	host := "mailpit"
	if transport.ServerName != "" {
		host = transport.ServerName
	}
	client, e := smtp.NewClient(conn, host)
	if e != nil {
		return e
	}
	defer client.Close()
	from := mail.Address{Name: "AI Business Assistant", Address: "no-reply@ai-business-assistant.test"}
	if transport.ServerName != "" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not advertise STARTTLS")
		}
		if e = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); e != nil {
			return e
		}
		if e = client.Auth(smtp.PlainAuth("", transport.Username, transport.Password, host)); e != nil {
			return e
		}
		from = transport.From
	}
	if e = client.Mail(from.Address); e != nil {
		return e
	}
	if e = client.Rcpt(recipient); e != nil {
		return e
	}
	body, e := renderIdentityEmail(from, recipient, msg)
	if e != nil {
		return e
	}
	writer, e := client.Data()
	if e != nil {
		return e
	}
	_, e = writer.Write(body)
	if e != nil {
		return e
	}
	if e = writer.Close(); e != nil {
		return e
	}
	// Closing DATA with a success reply is the SMTP delivery boundary.
	// A failed QUIT cannot turn an accepted message into a retry.
	_ = client.Quit()
	return nil
}
