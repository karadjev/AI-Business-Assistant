package agent

import (
	"context"
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	"time"
)

type budgetPermitKey struct{}
type Metered struct {
	Provider       Provider
	Ledger         costcontrol.Service
	Tenant         catalog.Tenant
	ConversationID string
}

func (p Metered) Mode() string { return p.Provider.Mode() }
func (p Metered) Generate(ctx context.Context, r Request) (Response, error) {
	if p.Mode() == "mock" {
		return p.Provider.Generate(ctx, r)
	}
	id, e := p.Ledger.Reserve(ctx, p.Tenant, p.ConversationID, r.InputTokens, r.OutputTokens, 1+p.Ledger.Config.Live.Retries)
	if e != nil {
		return Response{}, e
	}
	result, e := p.Provider.Generate(context.WithValue(ctx, budgetPermitKey{}, true), r)
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if settle := p.Ledger.Settle(finish, p.Tenant, id, result.InputTokens, result.OutputTokens, e == nil && result.UsageKnown); settle != nil {
		return Response{}, errors.New("budget reconciliation unavailable; reservation retained")
	}
	return result, e
}

type Denied struct{}

func (Denied) Mode() string { return "live" }
func (Denied) Generate(context.Context, Request) (Response, error) {
	return Response{}, errors.New("external LLM disabled or zero budget")
}
