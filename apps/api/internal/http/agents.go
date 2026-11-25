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
	register("GET", root+"/workshop/manager", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		out, err := s.ManagerChat(r.Context(), tenant(r, session))
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	register("POST", root+"/workshop/manager", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.ManagerMessage
		if !decode(w, r, &in) {
			return
		}
		if err := s.SendManagerMessage(r.Context(), tenant(r, session), in); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	register("GET", root+"/workshop", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		out, err := s.Workshop(r.Context(), tenant(r, session))
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 200, out)
	})
	register("POST", root+"/workshop/customers", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		id, err := s.CreateWorkshopCustomer(r.Context(), tenant(r, session), in.Name)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 201, map[string]string{"id": id})
	})
	register("POST", root+"/workshop/requests", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.WorkshopRequest
		if !decode(w, r, &in) {
			return
		}
		id, err := s.SubmitWorkshop(r.Context(), tenant(r, session), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"id": id})
	})
	register("POST", root+"/workshop/requests/{request}/marketing", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var input agentruntime.WorkshopMarketingInput
		if !decode(w, r, &input) {
			return
		}
		id, err := s.WorkshopMarketingAction(r.Context(), tenant(r, session), r.PathValue("request"), input)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
	})
	register("POST", root+"/workshop/requests/{request}/financials", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var input agentruntime.WorkshopFinancialInput
		if !decode(w, r, &input) {
			return
		}
		id, err := s.RecordWorkshopFinancials(r.Context(), tenant(r, session), r.PathValue("request"), input)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
	})
	register("POST", root+"/workshop/requests/{request}/close", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in struct {
			Operation string `json:"operation"`
		}
		if !decode(w, r, &in) {
			return
		}
		id, err := s.ProposeWorkshopClose(r.Context(), tenant(r, session), r.PathValue("request"), in.Operation)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
	})
	register("POST", root+"/workshop/approvals/{approval}/decision", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in struct {
			Decision string `json:"decision"`
			Hash     string `json:"parameters_hash"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.DecideWorkshop(r.Context(), tenant(r, session), r.PathValue("approval"), in.Decision, in.Hash); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
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
	register("POST", root+"/agent-delegations/real-estate", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.RealEstateRequest
		if !decode(w, r, &in) {
			return
		}
		id, err := s.SubmitRealEstate(r.Context(), tenant(r, session), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"id": id})
	})
	register("POST", root+"/agent-delegations/property-viewing", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in agentruntime.PropertyViewingRequest
		if !decode(w, r, &in) {
			return
		}
		id, err := s.SubmitPropertyViewing(r.Context(), tenant(r, session), in)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 202, map[string]string{"id": id})
	})
	register("POST", root+"/agent-delegations/property-viewing/{task}/prepare", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		out, err := s.PreparePropertyViewing(r.Context(), tenant(r, session), r.PathValue("task"))
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
