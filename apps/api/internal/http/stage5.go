package httpapi

import (
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"net/http"
	"strconv"
	"time"
)

func stage5Routes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), identity *auth.Service, chat *conversations.Service, decode func(http.ResponseWriter, *http.Request, any) bool, fail func(http.ResponseWriter, *http.Request, error)) {
	orderService := orders.Service{Pool: identity.Pool, StockFreshness: chat.Config.StockFreshness}
	businessOperations(register, decode, fail, catalog.Service{Pool: identity.Pool}, orderService, servicebooking.Service{Pool: identity.Pool})
	base := "/api/v1/organizations/{id}"
	register("POST", "/api/v1/organizations/test", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		if e := identity.Limit(r.Context(), "create-org:"+session.User.ID, 10, time.Hour); e != nil {
			fail(w, r, e)
			return
		}
		o, e := (organizations.Service{Pool: identity.Pool}).CreateTest(r.Context(), session, in.Name)
		if e != nil {
			fail(w, r, e)
			return
		}
		writeJSON(w, 201, o)
	})
	routes := []struct{ method, path, op string }{
		{"GET", "/orders", "orders"}, {"GET", "/orders/{resource}", "order"}, {"POST", "/orders/{resource}/confirm", "confirm"}, {"POST", "/orders/{resource}/reject", "reject"}, {"POST", "/orders/{resource}/cancel", "cancel"}, {"POST", "/orders/{resource}/complete", "complete"},
		{"GET", "/conversations", "conversations"}, {"POST", "/conversations", "create"}, {"GET", "/conversations/{resource}", "conversation"}, {"POST", "/conversations/{resource}/messages", "send"}, {"POST", "/conversations/{resource}/handoff", "handoff"}, {"POST", "/conversations/{resource}/resume", "resume"}, {"POST", "/conversations/{resource}/delete", "delete"}, {"POST", "/conversations/{resource}/quotes", "quote"}, {"POST", "/conversations/{resource}/confirm-request", "consent"}, {"GET", "/agent/status", "agent"}, {"PATCH", "/members/{resource}/order-access", "access"},
	}
	for _, route := range routes {
		register(route.method, base+route.path, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			t := catalog.Tenant{Session: session, OrganizationID: r.PathValue("id")}
			id := r.PathValue("resource")
			if e := identity.Limit(r.Context(), "stage5:"+session.User.ID, 120, time.Minute); e != nil {
				fail(w, r, e)
				return
			}
			page := 1
			q := r.URL.Query()
			for key, values := range q {
				if len(values) != 1 || (key != "page" && !(key == "channel" && route.op == "conversations") && !(key == "status" && route.op == "orders")) {
					fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Неизвестный параметр"))
					return
				}
			}
			if q.Has("page") {
				var e error
				page, e = strconv.Atoi(q.Get("page"))
				if e != nil || page < 1 || page > 10000 {
					fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Некорректная страница"))
					return
				}
			}
			var result any
			var e error
			status := 200
			switch route.op {
			case "orders":
				result, e = orderService.List(r.Context(), t, q.Get("status"), page)
			case "order":
				result, e = orderService.Get(r.Context(), t, id)
			case "confirm", "reject", "cancel", "complete":
				var in orders.Transition
				if !decode(w, r, &in) {
					return
				}
				result, e = orderService.Change(r.Context(), t, id, route.op, in)
			case "conversations":
				e = chat.PurgeExpired(r.Context(), t)
				if e == nil {
					result, e = chat.ListChannel(r.Context(), t, page, q.Get("channel"))
				}
			case "create":
				var in struct{}
				if !decode(w, r, &in) {
					return
				}
				result, e = chat.Create(r.Context(), t)
				status = 201
			case "conversation":
				e = chat.PurgeExpired(r.Context(), t)
				if e == nil {
					result, e = chat.Get(r.Context(), t, id)
				}
			case "send":
				var in conversations.SendInput
				if !decode(w, r, &in) {
					return
				}
				result, e = chat.Send(r.Context(), t, id, in)
			case "handoff", "resume", "delete":
				var in struct {
					ExpectedVersion int64 `json:"expected_version"`
				}
				if !decode(w, r, &in) {
					return
				}
				if in.ExpectedVersion < 1 {
					fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Нужна версия диалога"))
					return
				}
				mode := map[string]string{"handoff": "human", "resume": "agent", "delete": "delete"}[route.op]
				result, e = chat.Control(r.Context(), t, id, mode, in.ExpectedVersion)
			case "quote":
				var in struct {
					Items []orders.CartItem `json:"items"`
				}
				if !decode(w, r, &in) {
					return
				}
				result, e = orderService.PrepareQuote(r.Context(), t, id, in.Items)
				status = 201
			case "consent":
				var in struct {
					QuoteID        string `json:"quote_id"`
					Accepted       bool   `json:"accepted"`
					IdempotencyKey string `json:"idempotency_key"`
				}
				if !decode(w, r, &in) {
					return
				}
				runner := agent.Runner{Catalog: catalog.Service{Pool: identity.Pool}, Orders: orderService, Tenant: t, ConversationID: id, Consent: &orders.Consent{QuoteID: in.QuoteID, ConversationID: id, Accepted: in.Accepted, IdempotencyKey: in.IdempotencyKey}}
				args, _ := json.Marshal(map[string]string{"quote_id": in.QuoteID})
				var executed agent.ToolResult
				executed, e = runner.Execute(r.Context(), "create_order_request", string(args))
				result = executed.Data
			case "agent":
				result, e = chat.State(r.Context(), t)
			case "access":
				var in struct {
					Allowed *bool `json:"allowed"`
				}
				if !decode(w, r, &in) {
					return
				}
				if in.Allowed == nil {
					fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Нужно allowed"))
					return
				}
				e = chat.SetOrderAccess(r.Context(), t, id, *in.Allowed)
				result = map[string]any{"allowed": *in.Allowed}
			}
			if e != nil {
				fail(w, r, e)
				return
			}
			writeJSON(w, status, result)
		})
	}
}
