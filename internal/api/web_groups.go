package api

import (
	"net/http"
	"strconv"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleWebGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListMonitorGroups(r.Context())
	if err != nil {
		s.logger.Error("web: list groups", "error", err)
	}

	pd := s.newPageData(r, "Groups", "groups")
	pd.Data = groups
	s.render(w, "groups.html", pd)
}

func (s *Server) handleWebGroupCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	g := &storage.MonitorGroup{
		Name: r.FormValue("name"),
	}
	if v := r.FormValue("sort_order"); v != "" {
		g.SortOrder, _ = strconv.Atoi(v)
	}

	if err := validateMonitorGroup(g); err != nil {
		s.setFlash(w, err.Error())
		s.redirect(w, r, "/groups")
		return
	}

	if err := s.store.CreateMonitorGroup(r.Context(), g); err != nil {
		s.logger.Error("web: create group", "error", err)
		s.setFlash(w, "Failed to create group")
		s.redirect(w, r, "/groups")
		return
	}

	s.setFlash(w, "Group created")
	s.redirect(w, r, "/groups")
}

func (s *Server) handleWebGroupUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	r.ParseForm()
	g := &storage.MonitorGroup{
		ID:   id,
		Name: r.FormValue("name"),
	}
	if v := r.FormValue("sort_order"); v != "" {
		g.SortOrder, _ = strconv.Atoi(v)
	}

	if err := validateMonitorGroup(g); err != nil {
		s.setFlash(w, err.Error())
		s.redirect(w, r, "/groups")
		return
	}

	if err := s.store.UpdateMonitorGroup(r.Context(), g); err != nil {
		s.logger.Error("web: update group", "error", err)
		s.setFlash(w, "Failed to update group")
		s.redirect(w, r, "/groups")
		return
	}

	s.setFlash(w, "Group updated")
	s.redirect(w, r, "/groups")
}

func (s *Server) handleWebGroupDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	if err := s.store.DeleteMonitorGroup(r.Context(), id); err != nil {
		s.logger.Error("web: delete group", "error", err)
	}
	s.setFlash(w, "Group deleted")
	s.redirect(w, r, "/groups")
}
