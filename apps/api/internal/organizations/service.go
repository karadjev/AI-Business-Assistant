package organizations

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

type Organization struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	BusinessType    string `json:"business_type"`
	Timezone        string `json:"timezone"`
	Version         int64  `json:"version"`
	Role            string `json:"role"`
	IsTest          bool   `json:"is_test"`
	CanManageOrders bool   `json:"can_manage_orders"`
}
type Service struct{ Pool *pgxpool.Pool }

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ValidID(id string) bool { return uuidRE.MatchString(id) }
func (s Service) transaction(ctx context.Context, session auth.Session, id string) (pgx.Tx, error) {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, auth.Unavailable()
	}
	_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(session.Hash), id)
	if e != nil {
		_ = tx.Rollback(ctx)
		return nil, auth.Unavailable()
	}
	return tx, nil
}
func (s Service) List(ctx context.Context, session auth.Session) ([]Organization, error) {
	tx, e := s.transaction(ctx, session, "")
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `SELECT o.id,o.name,o.business_type,o.timezone,o.version,o.role,a.is_test,a.can_manage_orders FROM platform.list_organizations() o JOIN platform.list_organization_access() a USING(id)`)
	if e != nil {
		return nil, auth.Unavailable()
	}
	defer rows.Close()
	result := []Organization{}
	for rows.Next() {
		var o Organization
		if e = rows.Scan(&o.ID, &o.Name, &o.BusinessType, &o.Timezone, &o.Version, &o.Role, &o.IsTest, &o.CanManageOrders); e != nil {
			return nil, auth.Unavailable()
		}
		result = append(result, o)
	}
	if rows.Err() != nil {
		return nil, auth.Unavailable()
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, auth.Unavailable()
	}
	return result, nil
}
func validate(name, timezone string) error {
	if strings.ContainsRune(name, 0) || utf8.RuneCountInString(strings.TrimSpace(name)) < 1 || utf8.RuneCountInString(name) > 120 {
		return auth.Failure(422, "VALIDATION_ERROR", "Название: 1–120 символов")
	}
	if _, e := time.LoadLocation(timezone); e != nil || timezone == "" || len(timezone) > 100 {
		return auth.Failure(422, "VALIDATION_ERROR", "Некорректный часовой пояс")
	}
	return nil
}
func (s Service) Create(ctx context.Context, session auth.Session, name, timezone string) (Organization, error) {
	return s.create(ctx, session, name, timezone, false)
}
func (s Service) CreateTest(ctx context.Context, session auth.Session, name string) (Organization, error) {
	return s.create(ctx, session, name, "Europe/Moscow", true)
}
func (s Service) create(ctx context.Context, session auth.Session, name, timezone string, test bool) (Organization, error) {
	name = strings.TrimSpace(name)
	if timezone == "" {
		timezone = "Europe/Moscow"
	}
	if e := validate(name, timezone); e != nil {
		return Organization{}, e
	}
	id := auth.UUID()
	tx, e := s.transaction(ctx, session, id)
	if e != nil {
		return Organization{}, e
	}
	defer tx.Rollback(ctx)
	fn := "platform.create_organization"
	if test {
		fn = "platform.create_test_organization"
	}
	if _, e = tx.Exec(ctx, `SELECT `+fn+`($1,$2,$3)`, id, name, timezone); e != nil {
		return Organization{}, auth.Unavailable()
	}
	if e = tx.Commit(ctx); e != nil {
		return Organization{}, auth.Unavailable()
	}
	return Organization{ID: id, Name: name, BusinessType: "electronics", Timezone: timezone, Version: 1, Role: "owner", IsTest: test, CanManageOrders: true}, nil
}
func (s Service) Get(ctx context.Context, session auth.Session, id string) (Organization, error) {
	if !ValidID(id) {
		return Organization{}, auth.Failure(404, "NOT_FOUND", "Организация не найдена")
	}
	tx, e := s.transaction(ctx, session, id)
	if e != nil {
		return Organization{}, e
	}
	defer tx.Rollback(ctx)
	var o Organization
	e = tx.QueryRow(ctx, `SELECT o.id,o.name,o.business_type,o.timezone,o.version,m.role,o.is_test,(m.role='owner' OR m.can_manage_orders) FROM platform.organizations o JOIN platform.organization_members m ON m.organization_id=o.id WHERE o.id=$1`, id).Scan(&o.ID, &o.Name, &o.BusinessType, &o.Timezone, &o.Version, &o.Role, &o.IsTest, &o.CanManageOrders)
	if errors.Is(e, pgx.ErrNoRows) {
		return Organization{}, auth.Failure(404, "NOT_FOUND", "Организация не найдена")
	}
	if e != nil {
		return Organization{}, auth.Unavailable()
	}
	return o, nil
}
func (s Service) Update(ctx context.Context, session auth.Session, id, name, timezone string, version int64) (Organization, error) {
	o, e := s.Get(ctx, session, id)
	if e != nil {
		return o, e
	}
	if o.Role != "owner" {
		return Organization{}, auth.Failure(403, "FORBIDDEN", "Только владелец может изменять настройки")
	}
	name = strings.TrimSpace(name)
	if e = validate(name, timezone); e != nil {
		return Organization{}, e
	}
	tx, e := s.transaction(ctx, session, id)
	if e != nil {
		return Organization{}, e
	}
	defer tx.Rollback(ctx)
	e = tx.QueryRow(ctx, `UPDATE platform.organizations SET name=$2,timezone=$3,version=version+1,updated_at=now() WHERE id=$1 AND version=$4 RETURNING version`, id, name, timezone, version).Scan(&o.Version)
	if errors.Is(e, pgx.ErrNoRows) {
		return Organization{}, auth.Failure(409, "VERSION_CONFLICT", "Настройки изменились. Обновите страницу")
	}
	if e != nil {
		return Organization{}, auth.Unavailable()
	}
	if e = tx.Commit(ctx); e != nil {
		return Organization{}, auth.Unavailable()
	}
	o.Name = name
	o.Timezone = timezone
	return o, nil
}
