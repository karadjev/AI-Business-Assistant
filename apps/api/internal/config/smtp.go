package config

import (
	"errors"
	"io"
	"net"
	"net/mail"
	"os"
	"strings"
)

type SMTP struct {
	ServerName string
	Username   string
	Password   string
	From       mail.Address
}

func loadSMTP(get func(string) string, addr string, secure bool) (SMTP, error) {
	if get("SMTP_PASSWORD") != "" {
		return SMTP{}, errors.New("SMTP password must use a mounted secret file")
	}
	mode := get("SMTP_MODE")
	if mode == "" || mode == "mailpit" {
		for _, key := range []string{"SMTP_TLS_SERVER_NAME", "SMTP_USERNAME", "SMTP_PASSWORD_FILE", "SMTP_FROM_EMAIL", "SMTP_FROM_NAME"} {
			if get(key) != "" {
				return SMTP{}, errors.New("SMTP credentials and sender require starttls mode")
			}
		}
		if addr != "mailpit:1025" && addr != "127.0.0.1:1025" && addr != "localhost:1025" {
			return SMTP{}, errors.New("SMTP_ADDR must use the local dev email sink")
		}
		return SMTP{}, nil
	}
	if mode != "starttls" || !secure {
		return SMTP{}, errors.New("authenticated SMTP requires starttls mode and an HTTPS runtime")
	}
	host, port, err := net.SplitHostPort(addr)
	server := get("SMTP_TLS_SERVER_NAME")
	// A private TCP gateway may provide the egress route. The TLS peer still
	// must present a certificate for the approved upstream hostname.
	private := net.ParseIP(host)
	if err != nil || port != "587" || server != "smtp-relay.brevo.com" ||
		(host != server && (private == nil || !private.IsPrivate())) {
		return SMTP{}, errors.New("SMTP requires the approved Brevo server and port")
	}
	user, name, email := get("SMTP_USERNAME"), get("SMTP_FROM_NAME"), get("SMTP_FROM_EMAIL")
	sender, err := mail.ParseAddress(email)
	if user == "" || len(user) > 254 || strings.ContainsAny(user, "\x00\r\n \t") ||
		name == "" || len(name) > 100 || strings.ContainsAny(name, "\x00\r\n") ||
		err != nil || sender.Address != email || sender.Name != "" {
		return SMTP{}, errors.New("SMTP username or sender is invalid")
	}
	password, err := readSMTPPassword(get("SMTP_PASSWORD_FILE"))
	if err != nil {
		return SMTP{}, err
	}
	return SMTP{ServerName: server, Username: user, Password: password, From: mail.Address{Name: name, Address: email}}, nil
}

func readSMTPPassword(path string) (string, error) {
	invalid := errors.New("SMTP password file must be a private regular file with one nonempty line")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", invalid
	}
	f, err := os.Open(path)
	if err != nil {
		return "", invalid
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) || actual.Mode().Perm()&0077 != 0 {
		return "", invalid
	}
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	value := strings.TrimSuffix(string(b), "\n")
	if err != nil || len(b) > 4096 || value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", invalid
	}
	return value, nil
}
