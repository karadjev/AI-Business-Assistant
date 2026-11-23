package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/instagram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func identityRoutes(mux *http.ServeMux, s *auth.Service, chat *conversations.Service, tg *telegram.Service, wa *whatsapp.Service, igs ...*instagram.Service) {
	orgs := organizations.Service{Pool: s.Pool}
	fail := func(w http.ResponseWriter, r *http.Request, e error) {
		var ae *auth.Error
		if errors.As(e, &ae) {
			if ae.Status == 429 {
				w.Header().Set("Retry-After", "60")
			}
			writeError(w, r, ae.Status, ae.Code, ae.Message)
		} else {
			writeError(w, r, 503, "SERVICE_UNAVAILABLE", "Сервис временно недоступен")
		}
	}
	cookieName := "aba_session"
	if s.Config.Secure {
		cookieName = "__Host-aba_session"
	}
	token := func(r *http.Request) string {
		c, e := r.Cookie(cookieName)
		if e != nil {
			return ""
		}
		return c.Value
	}
	decode := func(w http.ResponseWriter, r *http.Request, target any) bool {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, r, 415, "UNSUPPORTED_MEDIA_TYPE", "Требуется application/json")
			return false
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		e := d.Decode(target)
		if e == nil {
			var tail any
			if d.Decode(&tail) != io.EOF {
				e = errors.New("trailing data")
			}
		}
		if e != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(e, &tooLarge) {
				writeError(w, r, 413, "BODY_TOO_LARGE", "Слишком большой запрос")
			} else {
				writeError(w, r, 400, "INVALID_JSON", "Некорректный JSON")
			}
			return false
		}
		return true
	}
	register := func(path, method string, protected bool, h func(http.ResponseWriter, *http.Request, auth.Session)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != method {
				w.Header().Set("Allow", method)
				writeError(w, r, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
				return
			}
			if method != "GET" && r.Header.Get("Origin") != s.Config.Origin {
				writeError(w, r, 403, "CSRF_REJECTED", "Источник запроса не разрешён")
				return
			}
			if s.Config.EmailActionsDisabled {
				switch path {
				case "/api/v1/auth/register", "/api/v1/auth/resend-verification", "/api/v1/auth/verify-email", "/api/v1/auth/password-reset", "/api/v1/auth/password-reset/confirm":
					writeError(w, r, 403, "EMAIL_ACTIONS_DISABLED", "Регистрация и восстановление доступа временно недоступны")
					return
				}
			}
			var session auth.Session
			if protected {
				var e error
				session, e = s.Authenticate(r.Context(), token(r))
				if e != nil {
					fail(w, r, e)
					return
				}
				if method != "GET" && !s.CSRF(session, r.Header.Get("X-CSRF-Token")) {
					writeError(w, r, 403, "CSRF_REJECTED", "Повторите действие после обновления страницы")
					return
				}
			}
			h(w, r, session)
		})
	}
	publicLimit := func(w http.ResponseWriter, r *http.Request, scope, key string, n int) bool {
		ip, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil {
			ip = r.RemoteAddr
		}
		if e = s.Limit(r.Context(), scope+":ip:"+ip, 40, time.Minute); e == nil {
			e = s.Limit(r.Context(), scope+":account:"+strings.ToLower(strings.TrimSpace(key)), n, time.Minute)
		}
		if e != nil {
			fail(w, r, e)
			return false
		}
		return true
	}
	message := map[string]string{"message": "Если адрес может быть зарегистрирован, письмо с подтверждением будет отправлено. Проверьте почту."}
	register("/api/v1/auth/register", "POST", false, func(w http.ResponseWriter, r *http.Request, _ auth.Session) {
		var in struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
		}
		if !decode(w, r, &in) || !publicLimit(w, r, "register", in.Email, 3) {
			return
		}
		if e := s.Register(r.Context(), in.Email, in.Password, in.DisplayName); e != nil {
			fail(w, r, e)
			return
		}
		writeJSON(w, 202, message)
	})
	register("/api/v1/auth/resend-verification", "POST", false, func(w http.ResponseWriter, r *http.Request, _ auth.Session) {
		var in struct {
			Email string `json:"email"`
		}
		if !decode(w, r, &in) || !publicLimit(w, r, "resend", in.Email, 3) {
			return
		}
		if e := s.Resend(r.Context(), in.Email); e != nil {
			fail(w, r, e)
			return
		}
		writeJSON(w, 202, message)
	})
	register("/api/v1/auth/password-reset", "POST", false, func(w http.ResponseWriter, r *http.Request, _ auth.Session) {
		var in struct {
			Email string `json:"email"`
		}
		if !decode(w, r, &in) || !publicLimit(w, r, "password-reset", in.Email, 3) {
			return
		}
		if e := s.RequestPasswordReset(r.Context(), in.Email); e != nil {
			fail(w, r, e)
			return
		}
		writeJSON(w, 202, map[string]string{"message": "Если аккаунт существует и email подтверждён, письмо для восстановления будет отправлено."})
	})
	register("/api/v1/auth/password-reset/confirm", "POST", false, func(w http.ResponseWriter, r *http.Request, _ auth.Session) {
		var in struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if !decode(w, r, &in) || !publicLimit(w, r, "password-reset-confirm", in.Token, 5) {
			return
		}
		if e := s.ResetPassword(r.Context(), in.Token, in.Password); e != nil {
			fail(w, r, e)
			return
		}
		w.WriteHeader(204)
	})
	register("/api/v1/auth/verify-email", "POST", false, func(w http.ResponseWriter, r *http.Request, _ auth.Session) {
		var in struct {
			Token string `json:"token"`
		}
		if !decode(w, r, &in) || !publicLimit(w, r, "verify", "all", 30) {
			return
		}
		if e := s.Verify(r.Context(), in.Token); e != nil {
			fail(w, r, e)
			return
		}
		w.WriteHeader(204)
	})
	register("/api/v1/auth/login", "POST", false, func(w http.ResponseWriter, r *http.Request, _ auth.Session) {
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decode(w, r, &in) || !publicLimit(w, r, "login", in.Email, 5) {
			return
		}
		user, t, c, e := s.Login(r.Context(), in.Email, in.Password, token(r))
		if e != nil {
			fail(w, r, e)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: t, Path: "/", HttpOnly: true, Secure: s.Config.Secure, SameSite: http.SameSiteLaxMode, MaxAge: int(s.Config.SessionTTL.Seconds())})
		http.SetCookie(w, &http.Cookie{Name: "aba_csrf", Value: c, Path: "/", Secure: s.Config.Secure, SameSite: http.SameSiteStrictMode, MaxAge: int(s.Config.SessionTTL.Seconds())})
		writeJSON(w, 200, map[string]any{"user": user})
	})
	register("/api/v1/auth/me", "GET", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		writeJSON(w, 200, map[string]any{"user": session.User})
	})
	register("/api/v1/auth/logout", "POST", true, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		if e := s.Logout(r.Context(), session); e != nil {
			fail(w, r, e)
			return
		}
		for _, name := range []string{cookieName, "aba_csrf"} {
			http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: name == cookieName, Secure: s.Config.Secure, SameSite: http.SameSiteLaxMode})
		}
		w.WriteHeader(204)
	})
	// Resource collection supports GET and POST with the same authentication/CSRF wrapper.
	dispatch := http.NewServeMux()
	registerCollection := func(method string, h func(http.ResponseWriter, *http.Request, auth.Session)) {
		tmp := http.NewServeMux()
		old := mux
		mux = tmp
		register("/api/v1/organizations", method, true, h)
		mux = old
		dispatch.Handle(method+" /api/v1/organizations", tmp)
	}
	registerCollection("GET", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		items, e := orgs.List(r.Context(), session)
		if e != nil {
			fail(w, r, e)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	})
	registerCollection("POST", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in struct {
			Name     string `json:"name"`
			Timezone string `json:"timezone"`
		}
		if !decode(w, r, &in) {
			return
		}
		if e := s.Limit(r.Context(), "create-org:"+session.User.ID, 10, time.Hour); e != nil {
			fail(w, r, e)
			return
		}
		o, e := orgs.Create(r.Context(), session, in.Name, in.Timezone)
		if e != nil {
			fail(w, r, e)
			return
		}
		writeJSON(w, 201, o)
	})
	mux.HandleFunc("/api/v1/organizations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "POST" {
			w.Header().Set("Allow", "GET, POST")
			writeError(w, r, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
			return
		}
		dispatch.ServeHTTP(w, r)
	})
	detail := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if r.Method != "GET" && r.Method != "PATCH" {
			w.Header().Set("Allow", "GET, PATCH")
			writeError(w, r, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
			return
		}
		session, e := s.Authenticate(r.Context(), token(r))
		if e != nil {
			fail(w, r, e)
			return
		}
		if r.Method == "PATCH" && (r.Header.Get("Origin") != s.Config.Origin || !s.CSRF(session, r.Header.Get("X-CSRF-Token"))) {
			writeError(w, r, 403, "CSRF_REJECTED", "Запрос отклонён")
			return
		}
		var o organizations.Organization
		if r.Method == "GET" {
			o, e = orgs.Get(r.Context(), session, id)
		} else {
			var in struct {
				Name            string `json:"name"`
				Timezone        string `json:"timezone"`
				ExpectedVersion int64  `json:"expected_version"`
			}
			if !decode(w, r, &in) {
				return
			}
			if in.ExpectedVersion < 1 {
				writeError(w, r, 422, "VALIDATION_ERROR", "expected_version must be positive")
				return
			}
			o, e = orgs.Update(r.Context(), session, id, in.Name, in.Timezone, in.ExpectedVersion)
		}
		if e != nil {
			fail(w, r, e)
			return
		}
		writeJSON(w, 200, o)
	}
	mux.HandleFunc("/api/v1/organizations/{id}", detail)
	catalogRoutes(func(method, path string, h func(http.ResponseWriter, *http.Request, auth.Session)) {
		tmp := http.NewServeMux()
		old := mux
		mux = tmp
		register(path, method, true, h)
		mux = old
		mux.Handle(method+" "+path, tmp)
	}, s, decode, fail)

	stage5Routes(func(method, path string, h func(http.ResponseWriter, *http.Request, auth.Session)) {
		tmp := http.NewServeMux()
		old := mux
		mux = tmp
		register(path, method, true, h)
		mux = old
		mux.Handle(method+" "+path, tmp)
	}, s, chat, decode, fail)

	if tg != nil {
		stage6Routes(func(method, path string, h func(http.ResponseWriter, *http.Request, auth.Session)) {
			tmp := http.NewServeMux()
			old := mux
			mux = tmp
			register(path, method, true, h)
			mux = old
			mux.Handle(method+" "+path, tmp)
		}, s, tg, decode, fail)
	}

	stage7Routes(func(method, path string, h func(http.ResponseWriter, *http.Request, auth.Session)) {
		tmp := http.NewServeMux()
		old := mux
		mux = tmp
		register(path, method, true, h)
		mux = old
		mux.Handle(method+" "+path, tmp)
	}, s, chat, tg, decode, fail)

	onboardingRoutes(func(method, path string, h func(http.ResponseWriter, *http.Request, auth.Session)) {
		tmp := http.NewServeMux()
		old := mux
		mux = tmp
		register(path, method, true, h)
		mux = old
		mux.Handle(method+" "+path, tmp)
	}, s, decode, fail)
	agentRoutes(func(method, path string, h func(http.ResponseWriter, *http.Request, auth.Session)) {
		old := mux
		mux = http.NewServeMux()
		register(path, method, true, h)
		old.Handle(method+" "+path, mux)
		mux = old
	}, agentruntime.Service{Pool: s.Pool, Key: s.Config.RetentionKey()}, decode, fail)

	if wa != nil {
		whatsappRoutes(func(method, path string, h func(http.ResponseWriter, *http.Request, auth.Session)) {
			tmp := http.NewServeMux()
			old := mux
			mux = tmp
			register(path, method, true, h)
			mux = old
			mux.Handle(method+" "+path, tmp)
		}, s, wa, decode, fail)
	}

	if len(igs) > 0 && igs[0] != nil {
		instagramRoutes(func(method, path string, h func(http.ResponseWriter, *http.Request, auth.Session)) {
			tmp := http.NewServeMux()
			old := mux
			mux = tmp
			register(path, method, true, h)
			mux = old
			mux.Handle(method+" "+path, tmp)
		}, s, igs[0], fail)
	}

}
