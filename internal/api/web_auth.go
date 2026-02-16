package api

import (
	"context"
	"net/http"
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
				Name:   sessionCookie,
				Value:  "",
				Path:   "/",
				MaxAge: -1,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyAPIKey, apiKey)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) webAdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := getAPIKey(r.Context())
		if k == nil || k.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
