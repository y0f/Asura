package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

const sessionCookie = "asura_session"

func hashSessionToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) handleWebLogin(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login.html", pageData{})
}

func (s *Server) handleWebLoginPost(w http.ResponseWriter, r *http.Request) {
	ip := extractIP(r)

	if !s.loginRL.allow(ip) {
		s.auditLogin("login_rate_limited", "", ip)
		s.render(w, "login.html", pageData{Error: "Too many login attempts. Try again later."})
		return
	}

	key := r.FormValue("api_key")
	if key == "" {
		s.render(w, "login.html", pageData{Error: "API key is required"})
		return
	}

	apiKey, ok := s.cfg.LookupAPIKey(key)
	if !ok {
		s.auditLogin("login_failed", "", ip)
		s.render(w, "login.html", pageData{Error: "Invalid API key"})
		return
	}

	token, err := generateSessionToken()
	if err != nil {
		s.logger.Error("generate session token", "error", err)
		s.render(w, "login.html", pageData{Error: "Internal error"})
		return
	}

	sess := &storage.Session{
		TokenHash:  hashSessionToken(token),
		APIKeyName: apiKey.Name,
		IPAddress:  ip,
		ExpiresAt:  time.Now().Add(s.cfg.Auth.Session.Lifetime),
	}
	if err := s.store.CreateSession(r.Context(), sess); err != nil {
		s.logger.Error("create session", "error", err)
		s.render(w, "login.html", pageData{Error: "Internal error"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.cfg.Auth.Session.Lifetime.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.Auth.Session.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	s.auditLogin("login_success", apiKey.Name, ip)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleWebLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		tokenHash := hashSessionToken(cookie.Value)
		s.store.DeleteSession(r.Context(), tokenHash)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.Auth.Session.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.Auth.Session.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) webAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		tokenHash := hashSessionToken(cookie.Value)
		sess, err := s.store.GetSessionByTokenHash(r.Context(), tokenHash)
		if err != nil {
			if err == sql.ErrNoRows {
				s.clearSessionCookie(w)
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			s.logger.Error("session lookup", "error", err)
			s.clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if time.Now().After(sess.ExpiresAt) {
			s.store.DeleteSession(r.Context(), tokenHash)
			s.clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		apiKey := s.cfg.LookupAPIKeyByName(sess.APIKeyName)
		if apiKey == nil {
			s.store.DeleteSession(r.Context(), tokenHash)
			s.clearSessionCookie(w)
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

func (s *Server) auditLogin(action, keyName, ip string) {
	s.store.InsertAudit(context.Background(), &storage.AuditEntry{
		Action:     action,
		Entity:     "session",
		APIKeyName: keyName,
		Detail:     ip,
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
