package agentruntime

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

const workshopFinanceManager = "delegate_workshop_finance"

func financialDelegation(action string) bool {
	return action == workshopFinanceManager || action == "delegation.workshop_finance" || action == "delegation.workshop_analytics"
}

type WorkshopFinancialInput struct {
	ServiceMinor  *int64 `json:"service_minor"`
	PartCostMinor *int64 `json:"part_cost_minor"`
	Paid          *bool  `json:"paid"`
}

func (s Service) RecordWorkshopFinancials(ctx context.Context, tenant catalog.Tenant, requestID string, in WorkshopFinancialInput) (string, error) {
	const maxMinor = 9_000_000_000_000
	if !organizations.ValidID(requestID) || in.ServiceMinor == nil || in.PartCostMinor == nil || in.Paid == nil ||
		*in.ServiceMinor < 0 || *in.ServiceMinor > maxMinor || *in.PartCostMinor < 0 || *in.PartCostMinor > maxMinor {
		return "", auth.Failure(422, "INVALID_FINANCIAL_RECORD", "Укажите подтверждённые суммы и статус оплаты")
	}
	if len(s.Key) != 32 {
		return "", auth.Unavailable()
	}
	tx, err := tenancy.Begin(ctx, s.Pool, tenant, true, false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id *string
	err = tx.QueryRow(ctx, `SELECT platform.workshop_record_financials($1,$2,$3,$4,$5,sha256($6::bytea))`,
		tenant.OrganizationID, requestID, *in.ServiceMinor, *in.PartCostMinor, *in.Paid, s.Proof()).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "54000" {
			return "", auth.Failure(429, "RATE_LIMITED", "Слишком много задач")
		}
		return "", tenancy.Error(err)
	}
	if id == nil {
		return "", auth.Failure(409, "FINANCIAL_RECORD_UNAVAILABLE", "Проверьте завершение заявки, сохранённые суммы и состояние отчёта")
	}
	if err = tx.Commit(ctx); err != nil {
		return "", tenancy.Error(err)
	}
	return *id, nil
}

type workshopBookingFinance struct {
	ServiceMinor int64 `json:"service_minor"`
	PartsMinor   int64 `json:"parts_minor"`
	CashMinor    int64 `json:"cash_minor"`
	GrossMinor   int64 `json:"gross_minor"`
	Paid         bool  `json:"paid"`
	Quantity     int   `json:"quantity"`
}

type workshopBusinessMetrics struct {
	CompletedCount int64 `json:"completed_count"`
	RecordedCount  int64 `json:"recorded_count"`
	PaidCount      int64 `json:"paid_count"`
	ServiceMinor   int64 `json:"service_minor"`
	PartsMinor     int64 `json:"parts_minor"`
	CashMinor      int64 `json:"cash_minor"`
	GrossMinor     int64 `json:"gross_minor"`
}

type workshopFinancialResult struct {
	Finance   *workshopBookingFinance  `json:"finance,omitempty"`
	Analytics *workshopBusinessMetrics `json:"analytics,omitempty"`
	AsOf      string                   `json:"as_of,omitempty"`
}

func runWorkshopFinance(action string, raw json.RawMessage) (delegationResult, error) {
	var ctx struct {
		workshopFinancialResult
		Children []delegationChild `json:"children"`
	}
	if err := json.Unmarshal(raw, &ctx); err != nil {
		return delegationResult{}, err
	}
	switch action {
	case "delegation.workshop_finance":
		f := ctx.Finance
		if f == nil || f.Quantity < 1 || f.ServiceMinor < 0 || f.PartsMinor < 0 {
			return delegationResult{}, af.ErrDenied
		}
		f.GrossMinor = f.ServiceMinor - f.PartsMinor
		if f.Paid {
			f.CashMinor = f.ServiceMinor
		}
		return delegationResult{State: "finance_ready", Financial: &workshopFinancialResult{Finance: f}}, nil
	case "delegation.workshop_analytics":
		a := ctx.Analytics
		if a == nil || a.PaidCount < 0 || a.RecordedCount < a.PaidCount || a.CompletedCount < a.RecordedCount ||
			a.ServiceMinor < 0 || a.PartsMinor < 0 || a.CashMinor < 0 || a.CashMinor > a.ServiceMinor {
			return delegationResult{}, af.ErrDenied
		}
		a.GrossMinor = a.ServiceMinor - a.PartsMinor
		return delegationResult{State: "analytics_ready", Financial: &workshopFinancialResult{Analytics: a}}, nil
	case workshopFinanceManager:
		fin := workshopChild(ctx.Children, "finance", "delegation.workshop_finance")
		metrics := workshopChild(ctx.Children, "analytics", "delegation.workshop_analytics")
		if len(ctx.Children) != 2 || fin == nil || fin.Financial == nil || fin.Financial.Finance == nil ||
			metrics == nil || metrics.Financial == nil || metrics.Financial.Analytics == nil {
			return delegationResult{State: "partial_failure", Children: ctx.Children, Message: "Финансовый отчёт подготовлен частично."}, nil
		}
		return delegationResult{State: "financial_report_ready", Children: ctx.Children,
			Financial: &workshopFinancialResult{Finance: fin.Financial.Finance, Analytics: metrics.Financial.Analytics, AsOf: ctx.AsOf},
			Message:   "Валовая прибыль по учтённым заявкам рассчитана как стоимость услуг минус себестоимость списанных деталей. В полученные деньги включены только подтверждённые оплаты."}, nil
	default:
		return delegationResult{}, af.ErrDenied
	}
}
