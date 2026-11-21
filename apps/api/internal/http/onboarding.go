package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
)

func onboardingRoutes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), identity *auth.Service, decode func(http.ResponseWriter, *http.Request, any) bool, fail func(http.ResponseWriter, *http.Request, error)) {
	s := catalog.Service{Pool: identity.Pool, CleanupProof: catalog.WorkerProof(identity.Config.RetentionKey())}
	base := "/api/v1/organizations/{id}"
	for _, route := range []struct{ method, path string }{{"GET", "/catalog-control"}, {"POST", "/catalog-control/retention-binding"}, {"PUT", "/catalog-control/policy"}, {"POST", "/catalog-control/source"}, {"POST", "/catalog-imports/{resource}/hold"}, {"GET", "/onboarding"}, {"PUT", "/store-profile"}, {"POST", "/onboarding"}, {"POST", "/catalog-imports"}, {"GET", "/catalog-imports/{resource}"}, {"POST", "/catalog-imports/{resource}/mapping"}, {"POST", "/catalog-imports/{resource}/confirm"}, {"POST", "/catalog-imports/{resource}/cancel"}} {
		register(route.method, base+route.path, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if e := identity.Limit(r.Context(), "onboarding:"+session.User.ID, 120, time.Minute); e != nil {
				fail(w, r, e)
				return
			}
			t := catalog.Tenant{Session: session, OrganizationID: r.PathValue("id")}
			var result any
			var e error
			switch route.path {
			case "/catalog-control":
				q := catalog.ControlQuery{Query: r.URL.Query().Get("q"), Filter: r.URL.Query().Get("filter")}
				for key, values := range r.URL.Query() {
					if len(values) != 1 || (key != "q" && key != "filter" && key != "page" && key != "import_page") {
						fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Неизвестный или повторный параметр"))
						return
					}
				}
				for key, target := range map[string]*int{"page": &q.Page, "import_page": &q.ImportPage} {
					if r.URL.Query().Has(key) {
						n, err := strconv.Atoi(r.URL.Query().Get(key))
						if err != nil || n < 1 {
							fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Некорректная страница"))
							return
						}
						*target = n
					}
				}
				if len(r.URL.Query()) == 0 {
					result, e = s.Control(r.Context(), t)
				} else {
					result, e = s.ControlPage(r.Context(), t, q)
				}
			case "/catalog-control/retention-binding":
				var in struct {
					Version *int64 `json:"expected_version"`
					Confirm bool   `json:"confirm"`
				}
				if !decode(w, r, &in) {
					return
				}
				if in.Version == nil {
					fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Укажите expected_version"))
					return
				}
				result, e = s.RebindCleanup(r.Context(), t, *in.Version, in.Confirm)
			case "/catalog-control/policy":
				var in catalog.Policy
				if !decode(w, r, &in) {
					return
				}
				result, e = s.SavePolicy(r.Context(), t, in)
			case "/catalog-control/source":
				var in catalog.SourceChange
				if !decode(w, r, &in) {
					return
				}
				result, e = s.ChangeSource(r.Context(), t, in)
			case "/catalog-imports/{resource}/hold":
				var in struct {
					Hold    bool  `json:"hold"`
					Version int64 `json:"expected_version"`
				}
				if !decode(w, r, &in) {
					return
				}
				result, e = s.ImportHold(r.Context(), t, r.PathValue("resource"), in.Version, in.Hold)

			case "/onboarding":
				if route.method == "GET" {
					result, e = s.Onboarding(r.Context(), t)
				} else {
					var in struct {
						Status  string `json:"status"`
						Version int64  `json:"expected_version"`
						Confirm bool   `json:"confirm"`
					}
					if !decode(w, r, &in) {
						return
					}
					result, e = s.OnboardingStep(r.Context(), t, in.Status, in.Version, in.Confirm)
				}
			case "/store-profile":
				var in catalog.StoreProfile
				if !decode(w, r, &in) {
					return
				}
				result, e = s.SaveProfile(r.Context(), t, in)
			case "/catalog-imports":
				b, err := io.ReadAll(io.LimitReader(r.Body, catalog.MaxXLSXBytes+1))
				if err != nil || len(b) > catalog.MaxXLSXBytes {
					writeError(w, r, 413, "BODY_TOO_LARGE", "Excel: максимум1MiB")
					return
				}
				result, e = s.UploadXLSX(r.Context(), t, r.Header.Get("X-Idempotency-Key"), b, r.Header.Get("X-Catalog-Filename"))
			case "/catalog-imports/{resource}":
				result, e = s.ImportResult(r.Context(), t, r.PathValue("resource"))
			case "/catalog-imports/{resource}/mapping":
				var in struct {
					Mapping map[string]int `json:"mapping"`
					Version int64          `json:"expected_version"`
				}
				if !decode(w, r, &in) {
					return
				}
				result, e = s.MapImport(r.Context(), t, r.PathValue("resource"), in.Version, in.Mapping)
			case "/catalog-imports/{resource}/confirm":
				var in struct {
					Version int64 `json:"expected_version"`
					Confirm bool  `json:"confirm"`
				}
				if !decode(w, r, &in) {
					return
				}
				result, e = s.ConfirmImport(r.Context(), t, r.PathValue("resource"), in.Version, in.Confirm)
			case "/catalog-imports/{resource}/cancel":
				var in struct {
					Version int64 `json:"expected_version"`
				}
				if !decode(w, r, &in) {
					return
				}
				result, e = s.CancelImport(r.Context(), t, r.PathValue("resource"), in.Version)
			}
			if e != nil {
				fail(w, r, e)
				return
			}
			writeJSON(w, 200, result)
		})
	}
}
