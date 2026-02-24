package web

import (
	"net/http"
	"strconv"

	"github.com/y0f/asura/internal/httputil"
	"github.com/y0f/asura/internal/storage"
	"github.com/y0f/asura/internal/validate"
)

func (h *Handler) Proxies(w http.ResponseWriter, r *http.Request) {
	proxies, err := h.store.ListProxies(r.Context())
	if err != nil {
		h.logger.Error("web: list proxies", "error", err)
	}

	pd := h.newPageData(r, "Proxies", "proxies")
	pd.Data = proxies
	h.render(w, "proxies/list.html", pd)
}

func (h *Handler) ProxyForm(w http.ResponseWriter, r *http.Request) {
	var proxy *storage.Proxy

	if idStr := r.PathValue("id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			h.redirect(w, r, "/proxies")
			return
		}
		proxy, err = h.store.GetProxy(r.Context(), id)
		if err != nil {
			h.redirect(w, r, "/proxies")
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

	pd := h.newPageData(r, title, "proxies")
	pd.Data = proxy
	h.render(w, "proxies/form.html", pd)
}

func (h *Handler) ProxyCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	p := parseProxyForm(r)

	if err := validate.ValidateProxy(p); err != nil {
		pd := h.newPageData(r, "New Proxy", "proxies")
		pd.Error = err.Error()
		pd.Data = p
		h.render(w, "proxies/form.html", pd)
		return
	}

	if err := h.store.CreateProxy(r.Context(), p); err != nil {
		h.logger.Error("web: create proxy", "error", err)
		h.setFlash(w, "Failed to create proxy")
		h.redirect(w, r, "/proxies")
		return
	}

	h.audit(r, "create", "proxy", p.ID, "")
	h.setFlash(w, "Proxy created")
	h.redirect(w, r, "/proxies")
}

func (h *Handler) ProxyUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		h.redirect(w, r, "/proxies")
		return
	}
	r.ParseForm()

	p := parseProxyForm(r)
	p.ID = id

	if err := validate.ValidateProxy(p); err != nil {
		pd := h.newPageData(r, "Edit Proxy", "proxies")
		pd.Error = err.Error()
		pd.Data = p
		h.render(w, "proxies/form.html", pd)
		return
	}

	if err := h.store.UpdateProxy(r.Context(), p); err != nil {
		h.logger.Error("web: update proxy", "error", err)
		h.setFlash(w, "Failed to update proxy")
		h.redirect(w, r, "/proxies")
		return
	}

	h.audit(r, "update", "proxy", p.ID, "")
	h.setFlash(w, "Proxy updated")
	h.redirect(w, r, "/proxies")
}

func (h *Handler) ProxyDelete(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		h.redirect(w, r, "/proxies")
		return
	}

	if err := h.store.DeleteProxy(r.Context(), id); err != nil {
		h.logger.Error("web: delete proxy", "error", err)
		h.setFlash(w, "Failed to delete proxy")
		h.redirect(w, r, "/proxies")
		return
	}

	h.audit(r, "delete", "proxy", id, "")
	h.setFlash(w, "Proxy deleted")
	h.redirect(w, r, "/proxies")
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
