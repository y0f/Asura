package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

const sessionCookie = "asura_session"

func (s *Server) handleWebLogin(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login.html", pageData{})
}

func (s *Server) handleWebLoginPost(w http.ResponseWriter, r *http.Request) {
	key := r.FormValue("api_key")
	if key == "" {
		s.render(w, "login.html", pageData{Error: "API key is required"})
		return
	}

	if _, ok := s.cfg.LookupAPIKey(key); !ok {
		s.render(w, "login.html", pageData{Error: "Invalid API key"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    key,
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleWebLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) webAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		apiKey, ok := s.cfg.LookupAPIKey(cookie.Value)
		if !ok {
			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookie,
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyAPIKey, apiKey)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) webRequirePerm(perm string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := getAPIKey(r.Context())
		if k == nil || !k.HasPermission(perm) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && !checkOrigin(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func checkOrigin(r *http.Request) bool {
	hosts := make(map[string]bool)
	hosts[stripPort(r.Host)] = true
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		hosts[stripPort(fwd)] = true
	}

	origin := r.Header.Get("Origin")
	if origin != "" && origin != "null" {
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return hosts[stripPort(u.Host)]
	}
	ref := r.Header.Get("Referer")
	if ref != "" {
		u, err := url.Parse(ref)
		if err != nil {
			return false
		}
		return hosts[stripPort(u.Host)]
	}
	return true
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i != -1 {
		return strings.ToLower(host[:i])
	}
	return strings.ToLower(host)
}
