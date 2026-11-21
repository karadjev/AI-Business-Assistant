// Package telegram adapts private Telegram chats to the existing agent.
package telegram

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Bot struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}
type SendResult struct {
	MessageID  int64
	RetryAfter int
	Uncertain  bool
}
type Transport interface {
	GetMe(context.Context, string) (Bot, error)
	Send(context.Context, string, int64, json.RawMessage) (SendResult, error)
}

// Mock accepts only explicitly synthetic credentials; it never performs network I/O.
type Mock struct{}

func (Mock) GetMe(_ context.Context, token string) (Bot, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 2 || parts[0] != "mock" {
		return Bot{}, errors.New("invalid synthetic bot credential")
	}
	id, e := strconv.ParseInt(parts[1], 10, 64)
	if e != nil || id < 1 {
		return Bot{}, errors.New("invalid synthetic bot id")
	}
	return Bot{id, "Синтетический бот", fmt.Sprintf("mock_shop_%d_bot", id)}, nil
}
func (Mock) Send(ctx context.Context, token string, chat int64, p json.RawMessage) (SendResult, error) {
	if e := ctx.Err(); e != nil {
		return SendResult{}, e
	}
	if _, e := (Mock{}).GetMe(ctx, token); e != nil {
		return SendResult{}, e
	}
	if chat < 1 || !json.Valid(p) {
		return SendResult{}, errors.New("invalid message")
	}
	return SendResult{MessageID: 1}, nil
}
func encrypt(key []byte, plain string, org string) ([]byte, error) {
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, errors.New("encryption unavailable")
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return g.Seal(nonce, nonce, []byte(plain), []byte(org)), nil
}
func decrypt(key, data []byte, org string) (string, error) {
	block, e := aes.NewCipher(key)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(block)
	if e != nil || len(data) < g.NonceSize() {
		return "", errors.New("invalid ciphertext")
	}
	raw, e := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], []byte(org))
	if e != nil {
		return "", errors.New("decryption unavailable")
	}
	return string(raw), nil
}

type User struct {
	ID    int64 `json:"id"`
	IsBot bool  `json:"is_bot"`
}
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
type Message struct {
	ID   int64  `json:"message_id"`
	From User   `json:"from"`
	Chat Chat   `json:"chat"`
	Text string `json:"text"`
}
type Callback struct {
	ID      string  `json:"id"`
	From    User    `json:"from"`
	Message Message `json:"message"`
	Data    string  `json:"data"`
}
type Update struct {
	ID       int64     `json:"update_id"`
	Message  *Message  `json:"message,omitempty"`
	Callback *Callback `json:"callback_query,omitempty"`
}

func (u Update) Principal() (int64, int64, error) {
	if u.ID < 0 || (u.Message == nil) == (u.Callback == nil) {
		return 0, 0, errors.New("unsupported update")
	}
	m := u.Message
	from := User{}
	if m != nil {
		from = m.From
		if len([]rune(m.Text)) < 1 || len([]rune(m.Text)) > 2000 || strings.ContainsRune(m.Text, 0) {
			return 0, 0, errors.New("invalid text")
		}
	} else {
		m = &u.Callback.Message
		from = u.Callback.From
		if u.Callback.ID == "" || len(u.Callback.ID) > 128 || len(u.Callback.Data) > 64 {
			return 0, 0, errors.New("invalid callback")
		}
	}
	if m.Chat.Type != "private" || from.IsBot || from.ID < 1 || m.Chat.ID != from.ID {
		return 0, 0, errors.New("private human chats only")
	}
	return from.ID, m.Chat.ID, nil
}
