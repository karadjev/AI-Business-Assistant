package httpapi

import (
	"net/http"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
)

func agentRoutes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), s agentruntime.Service,
	decode func(http.ResponseWriter, *http.Request, any) bool, fail func(http.ResponseWriter, *http.Request, error)) {
	const root = "/api/v1/organizations/{id}"
	tenant := func(r *http.Request, session auth.Session) catalog.Tenant {
		return catalog.Tenant{OrganizationID: r.PathValue("id"), Session: session}
	}
	register("GET", root+"/agent-team", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		team, err := s.Team(r.Context(), tenant(r, session))
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 200, team)
	})
	register("PATCH", root+"/agent-business-profile", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in struct {
			Model    string `json:"model"`
			Industry string `json:"industry"`
			Revision int    `json:"revision"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.UpdateProfile(r.Context(), tenant(r, session), in.Model, in.Industry, in.Revision); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	register("GET", root+"/agent-approvals", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		out, err := s.ListApprovals(r.Context(), tenant(r, session))
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 200, out)
	})
	register("POST", root+"/agent-approvals/{approval}/decision", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in struct {
			Decision string `json:"decision"`
			Hash     string `json:"parameters_hash"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.Decide(r.Context(), tenant(r, session), r.PathValue("approval"), in.Decision, in.Hash); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	register("POST", root+"/agent-tasks", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.Request
		if !decode(w, r, &in) {
			return
		}
		id, err := s.Submit(r.Context(), tenant(r, session), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"id": id})
	})
	register("POST", root+"/agent-delegations/auto-parts", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.DelegationRequest
		if !decode(w, r, &in) {
			return
		}
		id, err := s.SubmitAutoParts(r.Context(), tenant(r, session), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"id": id})
	})
	register("POST", root+"/agent-delegations/construction", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.IndustryDelegationRequest[agentruntime.ConstructionRequirements]
		if !decode(w, r, &in) {
			return
		}
		id, err := s.SubmitConstruction(r.Context(), tenant(r, session), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"id": id})
	})
	register("POST", root+"/agent-delegations/auto-service", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.IndustryDelegationRequest[agentruntime.ServiceRequirements]
		if !decode(w, r, &in) {
			return
		}
		id, err := s.SubmitService(r.Context(), tenant(r, session), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"id": id})
	})
	register("POST", root+"/agent-delegations/auto-service/{task}/prepare", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		out, err := s.PrepareDelegatedBooking(r.Context(), tenant(r, session), r.PathValue("task"))
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 201, out)
	})
	register("POST", root+"/agent-workflows/preview", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.WorkflowPreviewRequest
		if !decode(w, r, &in) {
			return
		}
		out, err := s.Preview(r.Context(), tenant(r, session), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 200, out)
	})
	register("GET", root+"/marketing-drafts", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		out, err := s.ListMarketingDrafts(r.Context(), tenant(r, session))
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 200, out)
	})
	register("POST", root+"/marketing-drafts/{draft}/review", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.MarketingDraftAction
		if !decode(w, r, &in) {
			return
		}
		if err := s.ChangeMarketingDraft(r.Context(), tenant(r, session), r.PathValue("draft"), in); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	register("GET", root+"/agent-tasks", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		out, err := s.ListTasks(r.Context(), tenant(r, session))
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 200, out)
	})
	register("GET", root+"/agent-audit", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		q := r.URL.Query()
		since := time.Now().AddDate(0, 0, -30)
		if value := q.Get("since"); value != "" {
			var err error
			since, err = time.Parse(time.RFC3339, value)
			if err != nil {
				fail(w, r, auth.Failure(422, "INVALID_FILTER", "Неверная дата"))
				return
			}
		}
		out, err := s.ListAudit(r.Context(), tenant(r, session), q.Get("task_id"), q.Get("agent_id"), q.Get("action"), since)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 200, out)
	})

}
