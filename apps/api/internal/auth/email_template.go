package auth

import (
	"bytes"
	"fmt"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
)

type identityEmail struct {
	Title, Description, Action, Link string
}

var identityEmailTemplate = template.Must(template.New("identity").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Title}}</title></head>
<body style="margin:0;background:#f6f5f2;color:#191923;font:16px/1.6 Arial,sans-serif">
<table role="presentation" width="100%" cellspacing="0" cellpadding="20"><tr><td align="center">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:560px;text-align:left;background:#fff;border:1px solid #dedce4;border-radius:16px">
<tr><td style="padding:24px 28px;background:#1c1f26;color:#f6f5f2;font-weight:bold;border-radius:15px 15px 0 0">AI Business Assistant</td></tr>
<tr><td style="padding:28px">
<h1 style="margin:0 0 16px;font-size:28px;line-height:1.2">{{.Title}}</h1>
<p style="margin:0 0 20px;color:#535363">{{.Description}}</p>
<p style="margin:0 0 20px"><a href="{{.Link}}" style="display:inline-block;padding:12px 18px;border-radius:8px;background:#55458f;color:#fff;text-decoration:none;font-weight:bold">{{.Action}}</a></p>
<p style="margin:0;color:#535363">Если вы не запрашивали это письмо, его можно проигнорировать.</p>
</td></tr></table></td></tr></table>
</body></html>`))

func renderIdentityEmail(from mail.Address, recipient string, msg emailPayload) ([]byte, error) {
	copy := identityEmail{
		Title: "Подтвердите email", Action: "Подтвердить email", Link: msg.Link,
		Description: "Откройте одноразовую ссылку, чтобы подтвердить адрес и войти в AI Business Assistant.",
	}
	if msg.Kind == "password_reset" {
		copy.Title, copy.Action = "Восстановите доступ", "Задать новый пароль"
		copy.Description = "Откройте одноразовую ссылку, чтобы задать новый пароль. Ссылка действует 30 минут."
	}

	var body bytes.Buffer
	parts := multipart.NewWriter(&body)
	for _, format := range []string{"text/plain", "text/html"} {
		part, err := parts.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {format + "; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		encoded := quotedprintable.NewWriter(part)
		if format == "text/plain" {
			_, err = fmt.Fprintf(encoded, "%s\r\n\r\n%s\r\n\r\n%s\r\n\r\nЕсли вы не запрашивали это письмо, его можно проигнорировать.\r\n", copy.Title, copy.Description, copy.Link)
		} else {
			err = identityEmailTemplate.Execute(encoded, copy)
		}
		if err != nil {
			return nil, err
		}
		if err := encoded.Close(); err != nil {
			return nil, err
		}
	}
	if err := parts.Close(); err != nil {
		return nil, err
	}

	var message bytes.Buffer
	fmt.Fprintf(&message, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n",
		from.String(), recipient, mime.QEncoding.Encode("UTF-8", copy.Title), parts.Boundary())
	message.Write(body.Bytes())
	return message.Bytes(), nil
}
