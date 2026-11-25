package httpapi

import (
	"net/http"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
)

func teamRoutes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), base string,
	identity *auth.Service, decode func(http.ResponseWriter, *http.Request, any) bool,
	fail func(http.ResponseWriter, *http.Request, error)) {
	service := organizations.Service{Pool: identity.Pool}
	for _, route := range []struct{ method, path string }{
		{"GET", "/team"}, {"GET", "/workspace-usage"}, {"POST", "/team"}, {"GET", "/commercial"}, {"PUT", "/commercial"},
	} {
		register(route.method, base+route.path, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if err := identity.Limit(r.Context(), "workspace:"+session.User.ID, 30, time.Minute); err != nil {
				fail(w, r, err)
				return
			}
			var result any
			var err error
			org := r.PathValue("id")
			if route.path == "/commercial" {
				var change *organizations.CommercialSettings
				if r.Method == "PUT" {
					change = new(organizations.CommercialSettings)
					if !decode(w, r, change) {
						return
					}
				}
				result, err = service.Commercial(r.Context(), session, org, change)
			} else if r.Method == "GET" {
				result, err = service.Team(r.Context(), session, org, route.path == "/workspace-usage")
			} else {
				var input organizations.TeamCommand
				if !decode(w, r, &input) {
					return
				}
				result, err = service.ManageTeam(r.Context(), session, org, input)
			}
			if err != nil {
				fail(w, r, err)
				return
			}
			writeJSON(w, 200, result)
		})
	}
}
