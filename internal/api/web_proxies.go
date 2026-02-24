package api

import (
	"net/http"
	"strconv"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleWebProxies(w http.ResponseWriter, r *http.Request) {
	proxies, err := s.store.ListProxies(r.Context())
	if err != nil {
		s.logger.Error("web: list proxies", "error", err)
	}

	pd := s.newPageData(r, "Proxies", "proxies")
	pd.Data = proxies
	s.render(w, "proxies.html", pd)
}

func (s *Server) handleWebProxyForm(w http.ResponseWriter, r *http.Request) {
	var proxy *storage.Proxy

	if idStr := r.PathValue("id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			s.redirect(w, r, "/proxies")
			return
		}
		proxy, err = s.store.GetProxy(r.Context(), id)
		if err != nil {
			s.redirect(w, r, "/proxies")
			return
		}
	}

	title := "New Proxy"
	if proxy != nil {
		title = "Edit Proxy"
	} else {
		proxy = &storage.Proxy{
			Protocol: "http",
			Port:     8080,
			Enabled:  true,
		}
	}

	pd := s.newPageData(r, title, "proxies")
	pd.Data = proxy
	s.render(w, "proxy_form.html", pd)
}

func (s *Server) handleWebProxyCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	p := parseProxyForm(r)

	if err := validateProxy(p); err != nil {
		pd := s.newPageData(r, "New Proxy", "proxies")
		pd.Error = err.Error()
		pd.Data = p
		s.render(w, "proxy_form.html", pd)
		return
	}

	if err := s.store.CreateProxy(r.Context(), p); err != nil {
		s.logger.Error("web: create proxy", "error", err)
		s.setFlash(w, "Failed to create proxy")
		s.redirect(w, r, "/proxies")
		return
	}

	s.audit(r, "create", "proxy", p.ID, "")
	s.setFlash(w, "Proxy created")
	s.redirect(w, r, "/proxies")
}

func (s *Server) handleWebProxyUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.redirect(w, r, "/proxies")
		return
	}
	r.ParseForm()

	p := parseProxyForm(r)
	p.ID = id

	if err := validateProxy(p); err != nil {
		pd := s.newPageData(r, "Edit Proxy", "proxies")
		pd.Error = err.Error()
		pd.Data = p
		s.render(w, "proxy_form.html", pd)
		return
	}

	if err := s.store.UpdateProxy(r.Context(), p); err != nil {
		s.logger.Error("web: update proxy", "error", err)
		s.setFlash(w, "Failed to update proxy")
		s.redirect(w, r, "/proxies")
		return
	}

	s.audit(r, "update", "proxy", p.ID, "")
	s.setFlash(w, "Proxy updated")
	s.redirect(w, r, "/proxies")
}

func (s *Server) handleWebProxyDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.redirect(w, r, "/proxies")
		return
	}

	if err := s.store.DeleteProxy(r.Context(), id); err != nil {
		s.logger.Error("web: delete proxy", "error", err)
		s.setFlash(w, "Failed to delete proxy")
		s.redirect(w, r, "/proxies")
		return
	}

	s.audit(r, "delete", "proxy", id, "")
	s.setFlash(w, "Proxy deleted")
	s.redirect(w, r, "/proxies")
}

func parseProxyForm(r *http.Request) *storage.Proxy {
	port, _ := strconv.Atoi(r.FormValue("port"))
	return &storage.Proxy{
		Name:     r.FormValue("name"),
		Protocol: r.FormValue("protocol"),
		Host:     r.FormValue("host"),
		Port:     port,
		AuthUser: r.FormValue("auth_user"),
		AuthPass: r.FormValue("auth_pass"),
		Enabled:  r.FormValue("enabled") == "on",
	}
}
