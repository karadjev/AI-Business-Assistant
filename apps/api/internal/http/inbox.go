package httpapi

import (
	"net/http"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
)

func inboxRoutes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), base string,
	identity *auth.Service, chat *conversations.Service, decode func(http.ResponseWriter, *http.Request, any) bool,
	fail func(http.ResponseWriter, *http.Request, error)) {
	for _, route := range []struct{ method, suffix string }{
		{"GET", "/inbox"}, {"POST", "/inbox"}, {"GET", "/conversations/{conversation}/inbox"},
		{"POST", "/conversations/{conversation}/inbox/{draft}"}, {"POST", "/conversations/{conversation}/inbox/assignee"},
	} {
		register(route.method, base+route.suffix, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if err := identity.Limit(r.Context(), "inbox:"+session.User.ID, 90, time.Minute); err != nil {
				fail(w, r, err)
				return
			}
			t := catalog.Tenant{Session: session, OrganizationID: r.PathValue("id")}
			conversation := r.PathValue("conversation")
			var result any
			var err error
			switch {
			case conversation == "":
				if r.Method == "POST" && !decode(w, r, &struct{}{}) {
					return
				}
				var enabled bool
				enabled, err = chat.InboxReview(r.Context(), t, r.Method == "POST")
				result = map[string]bool{"review_enabled": enabled}
			case r.Method == "GET":
				result, err = chat.InboxDetail(r.Context(), t, conversation)
			case route.suffix == "/conversations/{conversation}/inbox/assignee":
				var in struct {
					Assignee        *string `json:"assignee"`
					ExpectedVersion int64   `json:"expected_version"`
				}
				if !decode(w, r, &in) {
					return
				}
				err = chat.AssignInbox(r.Context(), t, conversation, in.Assignee, in.ExpectedVersion)
			default:
				var in struct {
					Decision string `json:"decision"`
				}
				if !decode(w, r, &in) {
					return
				}
				err = chat.DecideInboxReply(r.Context(), t, conversation, r.PathValue("draft"), in.Decision)
			}
			if err != nil {
				fail(w, r, err)
				return
			}
			if result == nil {
				w.WriteHeader(204)
				return
			}
			writeJSON(w, 200, result)
		})
	}
}
