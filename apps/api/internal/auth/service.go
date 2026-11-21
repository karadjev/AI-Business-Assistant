package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"strings"
	"time"
	"unicode/utf8"
)

type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string                       { return e.Code }
func Failure(status int, code, message string) error { return &Error{status, code, message} }
func Unavailable() error {
	return Failure(503, "SERVICE_UNAVAILABLE", "Сервис временно недоступен")
}

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}
type Session struct {
	User     User
	Hash     []byte
	CSRFHash []byte
}
type Service struct {
	Pool   *pgxpool.Pool
	Config config.Auth
	dummy  string
	slots  chan struct{}
}

func New(pool *pgxpool.Pool, c config.Auth) *Service {
	return &Service{Pool: pool, Config: c, dummy: PasswordHash("a-dummy-password-not-a-login"), slots: make(chan struct{}, 2)}
}
func (s *Service) Limit(ctx context.Context, key string, limit int, window time.Duration) error {
	var count int
	bucket := time.Now().UTC().Truncate(window)
	err := s.Pool.QueryRow(ctx, `INSERT INTO platform.auth_rate_limits(key_hash,bucket,attempts) VALUES($1,$2,1) ON CONFLICT(key_hash,bucket) DO UPDATE SET attempts=auth_rate_limits.attempts+1 RETURNING attempts`, Hash(key), bucket).Scan(&count)
	if err != nil {
		return Unavailable()
	}
	if count > limit {
		return Failure(429, "RATE_LIMITED", "Слишком много попыток. Повторите позже")
	}
	return nil
}
func (s *Service) Register(ctx context.Context, email, password, name string) error {
	normalized, e := Email(email)
	if e != nil {
		return e
	}
	name = strings.TrimSpace(name)
	if strings.ContainsRune(name, 0) || !PasswordValid(password) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 {
		return Failure(422, "VALIDATION_ERROR", "Пароль: 12–128 символов; имя: 1–100 символов")
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	hash := PasswordHash(password)
	<-s.slots
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return Unavailable()
	}
	defer tx.Rollback(ctx)
	id := UUID()
	tag, e := tx.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name) VALUES($1,$2,$3,$4) ON CONFLICT(email) DO NOTHING`, id, normalized, hash, name)
	if e != nil {
		return Unavailable()
	}
	if tag.RowsAffected() > 0 {
		if e = s.queueVerification(ctx, tx, id, normalized); e != nil {
			return e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return Unavailable()
	}
	return nil
}
func (s *Service) queueVerification(ctx context.Context, tx pgx.Tx, id, email string) error {
	token := Token()
	_, e := tx.Exec(ctx, `UPDATE platform.email_verification_tokens SET consumed_at=now() WHERE user_id=$1 AND consumed_at IS NULL`, id)
	if e != nil {
		return Unavailable()
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform.email_verification_tokens(id,user_id,token_hash,expires_at) VALUES($1,$2,$3,$4)`, UUID(), id, Hash(token), time.Now().Add(s.Config.VerificationTTL))
	if e != nil {
		return Unavailable()
	}
	plain, _ := json.Marshal(emailPayload{To: email, Link: s.Config.Origin + "/verify-email#token=" + token})
	payload, e := encrypt(s.Config.EmailKey, plain)
	if e != nil {
		return Unavailable()
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform.email_outbox(id,payload) VALUES($1,$2)`, UUID(), payload)
	if e != nil {
		return Unavailable()
	}
	return nil
}
func (s *Service) Resend(ctx context.Context, email string) error {
	normalized, e := Email(email)
	if e != nil {
		return e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return Unavailable()
	}
	defer tx.Rollback(ctx)
	var id string
	e = tx.QueryRow(ctx, `SELECT id FROM platform.users WHERE email=$1 AND email_verified_at IS NULL FOR UPDATE`, normalized).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return Unavailable()
	}
	if e = s.queueVerification(ctx, tx, id, normalized); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return Unavailable()
	}
	return nil
}
func (s *Service) Verify(ctx context.Context, token string) error {
	if len(token) != 43 {
		return Failure(422, "INVALID_TOKEN", "Ссылка недействительна или истекла")
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return Unavailable()
	}
	defer tx.Rollback(ctx)
	var id string
	e = tx.QueryRow(ctx, `UPDATE platform.email_verification_tokens SET consumed_at=now() WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>now() RETURNING user_id`, Hash(token)).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return Failure(422, "INVALID_TOKEN", "Ссылка недействительна или истекла")
	}
	if e != nil {
		return Unavailable()
	}
	_, e = tx.Exec(ctx, `UPDATE platform.users SET email_verified_at=coalesce(email_verified_at,now()) WHERE id=$1`, id)
	if e != nil {
		return Unavailable()
	}
	if e = tx.Commit(ctx); e != nil {
		return Unavailable()
	}
	return nil
}
func (s *Service) Login(ctx context.Context, email, password, previous string) (User, string, string, error) {
	normalized, e := Email(email)
	if e != nil || !PasswordValid(password) {
		return User{}, "", "", Failure(401, "INVALID_CREDENTIALS", "Неверные данные или email ещё не подтверждён")
	}
	var user User
	var hash string
	var verified bool
	e = s.Pool.QueryRow(ctx, `SELECT id,email,display_name,password_hash,email_verified_at IS NOT NULL FROM platform.users WHERE email=$1`, normalized).Scan(&user.ID, &user.Email, &user.DisplayName, &hash, &verified)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return User{}, "", "", Unavailable()
	}
	if errors.Is(e, pgx.ErrNoRows) {
		hash = s.dummy
	}
	if err := s.acquire(ctx); err != nil {
		return User{}, "", "", err
	}
	matches := PasswordMatches(password, hash)
	<-s.slots
	if !matches || !verified {
		return User{}, "", "", Failure(401, "INVALID_CREDENTIALS", "Неверные данные или email ещё не подтверждён")
	}
	token, csrf := Token(), Token()
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return User{}, "", "", Unavailable()
	}
	defer tx.Rollback(ctx)
	if previous != "" {
		if _, e = tx.Exec(ctx, `SELECT platform.revoke_session($1)`, Hash(previous)); e != nil {
			return User{}, "", "", Unavailable()
		}
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,$5,$6)`, UUID(), user.ID, Hash(token), Hash(csrf), time.Now().Add(s.Config.SessionTTL), time.Now().Add(s.Config.IdleTTL))
	if e != nil {
		return User{}, "", "", Unavailable()
	}
	if e = tx.Commit(ctx); e != nil {
		return User{}, "", "", Unavailable()
	}
	return user, token, csrf, nil
}
func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	if len(token) != 43 {
		return Session{}, Failure(401, "UNAUTHENTICATED", "Войдите в аккаунт")
	}
	session := Session{Hash: Hash(token)}
	e := s.Pool.QueryRow(ctx, `SELECT id,email,display_name,csrf_hash FROM platform.session_user($1)`, session.Hash).Scan(&session.User.ID, &session.User.Email, &session.User.DisplayName, &session.CSRFHash)
	if errors.Is(e, pgx.ErrNoRows) {
		return Session{}, Failure(401, "UNAUTHENTICATED", "Войдите в аккаунт")
	}
	if e != nil {
		return Session{}, Unavailable()
	}
	if _, e = s.Pool.Exec(ctx, `SELECT platform.touch_session($1,$2::interval)`, session.Hash, fmt.Sprintf("%f seconds", s.Config.IdleTTL.Seconds())); e != nil {
		return Session{}, Unavailable()
	}
	return session, nil
}
func (s *Service) CSRF(session Session, token string) bool {
	return len(token) == 43 && subtle.ConstantTimeCompare(Hash(token), session.CSRFHash) == 1
}
func (s *Service) Logout(ctx context.Context, session Session) error {
	if _, e := s.Pool.Exec(ctx, `SELECT platform.revoke_session($1)`, session.Hash); e != nil {
		return Unavailable()
	}
	return nil
}

func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return Unavailable()
	}
}
