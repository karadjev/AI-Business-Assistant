package auth

import (
	"bufio"
	"context"
	"crypto/tls"
	"net"
	"net/http/httptest"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
)

func TestSMTPRefusesMissingSTARTTLS(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	command := make(chan string, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			command <- "accept failed"
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		in := bufio.NewReader(conn)
		_, _ = conn.Write([]byte("220 loopback SMTP\r\n"))
		hello, _ := in.ReadString('\n')
		if !strings.HasPrefix(hello, "EHLO ") {
			command <- "EHLO missing"
			return
		}
		_, _ = conn.Write([]byte("250-loopback\r\n250 AUTH PLAIN\r\n"))
		next, _ := in.ReadString('\n')
		command <- next
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cfg := config.SMTP{ServerName: "smtp-relay.brevo.com", Username: "synthetic", Password: "synthetic", From: mail.Address{Address: "no-reply@example.test"}}
	err = send(ctx, l.Addr().String(), cfg, emailPayload{To: "owner@example.test"})
	if err == nil || err.Error() != "SMTP server does not advertise STARTTLS" {
		t.Fatal("server without STARTTLS was not refused")
	}
	if next := <-command; next != "" {
		t.Fatal("client continued SMTP after missing STARTTLS")
	}
}

func TestSMTPRefusesUntrustedCertificateBeforeAuth(t *testing.T) {
	testServer := httptest.NewTLSServer(nil)
	cert := testServer.TLS.Certificates[0]
	testServer.Close()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	upgraded := make(chan bool, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			upgraded <- false
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		in := bufio.NewReader(conn)
		_, _ = conn.Write([]byte("220 loopback SMTP\r\n"))
		_, _ = in.ReadString('\n')
		_, _ = conn.Write([]byte("250-loopback\r\n250 STARTTLS\r\n"))
		next, _ := in.ReadString('\n')
		if next != "STARTTLS\r\n" {
			upgraded <- false
			return
		}
		_, _ = conn.Write([]byte("220 upgrading\r\n"))
		server := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		upgraded <- server.Handshake() != nil
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cfg := config.SMTP{ServerName: "smtp-relay.brevo.com", Username: "synthetic", Password: "synthetic", From: mail.Address{Address: "no-reply@example.test"}}
	err = send(ctx, l.Addr().String(), cfg, emailPayload{To: "owner@example.test"})
	if err == nil || !strings.Contains(err.Error(), "failed to verify certificate") || !<-upgraded {
		t.Fatal("untrusted STARTTLS peer was not rejected")
	}
}
