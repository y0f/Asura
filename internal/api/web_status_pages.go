package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleWebStatusPages(w http.ResponseWriter, r *http.Request) {
	pages, err := s.store.ListStatusPages(r.Context())
	if err != nil {
		s.logger.Error("web: list status pages", "error", err)
	}

	pd := s.newPageData(r, "Status Pages", "status-pages")
	pd.Data = pages
	s.render(w, "status_pages.html", pd)
}

func (s *Server) handleWebStatusPageForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var sp *storage.StatusPage
	var pageMonitors []storage.StatusPageMonitor

	idStr := r.PathValue("id")
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			s.setFlash(w, "Invalid status page ID")
			s.redirect(w, r, "/status-pages")
			return
		}
		sp, err = s.store.GetStatusPage(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				s.setFlash(w, "Status page not found")
				s.redirect(w, r, "/status-pages")
				return
			}
			s.logger.Error("web: get status page", "error", err)
			s.setFlash(w, "Failed to load status page")
			s.redirect(w, r, "/status-pages")
			return
		}
		pageMonitors, err = s.store.ListStatusPageMonitors(ctx, id)
		if err != nil {
			s.logger.Error("web: list status page monitors", "error", err)
		}
	}

	// Fetch all monitors for the checklist
	allMonitors, err := s.store.ListMonitors(ctx, storage.MonitorListFilter{}, storage.Pagination{Page: 1, PerPage: 10000})
	if err != nil {
		s.logger.Error("web: list monitors for status page form", "error", err)
	}

	var monitors []*storage.Monitor
	if allMonitors != nil {
		monitors, _ = allMonitors.Data.([]*storage.Monitor)
	}
	if monitors == nil {
		monitors = []*storage.Monitor{}
	}

	// Build lookups for assigned monitors
	assignedSet := make(map[int64]bool, len(pageMonitors))
	assignedData := make(map[int64]storage.StatusPageMonitor, len(pageMonitors))
	for _, pm := range pageMonitors {
		assignedSet[pm.MonitorID] = true
		assignedData[pm.MonitorID] = pm
	}

	pd := s.newPageData(r, "Status Page", "status-pages")
	pd.Data = map[string]interface{}{
		"StatusPage":   sp,
		"Monitors":     monitors,
		"Assigned":     assignedSet,
		"AssignedData": assignedData,
	}
	s.render(w, "status_page_form.html", pd)
}

func (s *Server) handleWebStatusPageCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	sp := &storage.StatusPage{
		Title:         strings.TrimSpace(r.FormValue("title")),
		Slug:          r.FormValue("slug"),
		Description:   strings.TrimSpace(r.FormValue("description")),
		Enabled:       r.FormValue("enabled") == "on",
		APIEnabled:    r.FormValue("api_enabled") == "on",
		ShowIncidents: r.FormValue("show_incidents") == "on",
		CustomCSS:     r.FormValue("custom_css"),
	}
	if v := r.FormValue("sort_order"); v != "" {
		sp.SortOrder, _ = strconv.Atoi(v)
	}

	if err := validateStatusPage(sp); err != nil {
		s.setFlash(w, err.Error())
		s.redirect(w, r, "/status-pages/new")
		return
	}

	ctx := r.Context()

	existing, err := s.store.GetStatusPageBySlug(ctx, sp.Slug)
	if err == nil && existing != nil {
		s.setFlash(w, "Slug already in use")
		s.redirect(w, r, "/status-pages/new")
		return
	}

	if err := s.store.CreateStatusPage(ctx, sp); err != nil {
		s.logger.Error("web: create status page", "error", err)
		s.setFlash(w, "Failed to create status page")
		s.redirect(w, r, "/status-pages/new")
		return
	}

	monitors := parseStatusPageMonitors(r, sp.ID)
	if len(monitors) > 0 {
		if err := s.store.SetStatusPageMonitors(ctx, sp.ID, monitors); err != nil {
			s.logger.Error("web: set status page monitors", "error", err)
		}
	}

	s.refreshStatusSlugs()
	s.setFlash(w, "Status page created")
	s.redirect(w, r, "/status-pages")
}

func (s *Server) handleWebStatusPageUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.setFlash(w, "Invalid ID")
		s.redirect(w, r, "/status-pages")
		return
	}

	r.ParseForm()

	sp := &storage.StatusPage{
		ID:            id,
		Title:         strings.TrimSpace(r.FormValue("title")),
		Slug:          r.FormValue("slug"),
		Description:   strings.TrimSpace(r.FormValue("description")),
		Enabled:       r.FormValue("enabled") == "on",
		APIEnabled:    r.FormValue("api_enabled") == "on",
		ShowIncidents: r.FormValue("show_incidents") == "on",
		CustomCSS:     r.FormValue("custom_css"),
	}
	if v := r.FormValue("sort_order"); v != "" {
		sp.SortOrder, _ = strconv.Atoi(v)
	}

	if err := validateStatusPage(sp); err != nil {
		s.setFlash(w, err.Error())
		s.redirect(w, r, "/status-pages/"+strconv.FormatInt(id, 10)+"/edit")
		return
	}

	ctx := r.Context()

	existing, err := s.store.GetStatusPageBySlug(ctx, sp.Slug)
	if err == nil && existing != nil && existing.ID != id {
		s.setFlash(w, "Slug already in use")
		s.redirect(w, r, "/status-pages/"+strconv.FormatInt(id, 10)+"/edit")
		return
	}

	if err := s.store.UpdateStatusPage(ctx, sp); err != nil {
		s.logger.Error("web: update status page", "error", err)
		s.setFlash(w, "Failed to update status page")
		s.redirect(w, r, "/status-pages/"+strconv.FormatInt(id, 10)+"/edit")
		return
	}

	monitors := parseStatusPageMonitors(r, id)
	if err := s.store.SetStatusPageMonitors(ctx, id, monitors); err != nil {
		s.logger.Error("web: set status page monitors", "error", err)
	}

	s.refreshStatusSlugs()
	s.setFlash(w, "Status page updated")
	s.redirect(w, r, "/status-pages")
}

func (s *Server) handleWebStatusPageDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	if err := s.store.DeleteStatusPage(r.Context(), id); err != nil {
		s.logger.Error("web: delete status page", "error", err)
	}
	s.refreshStatusSlugs()
	s.setFlash(w, "Status page deleted")
	s.redirect(w, r, "/status-pages")
}

func parseStatusPageMonitors(r *http.Request, pageID int64) []storage.StatusPageMonitor {
	var result []storage.StatusPageMonitor
	for key, vals := range r.Form {
		if !strings.HasPrefix(key, "monitor_") || !strings.HasSuffix(key, "_enabled") {
			continue
		}
		if len(vals) == 0 || vals[0] != "on" {
			continue
		}
		idStr := strings.TrimSuffix(strings.TrimPrefix(key, "monitor_"), "_enabled")
		monitorID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		sortOrder := 0
		if v := r.FormValue("monitor_" + idStr + "_sort"); v != "" {
			sortOrder, _ = strconv.Atoi(v)
		}
		groupName := strings.TrimSpace(r.FormValue("monitor_" + idStr + "_group"))

		result = append(result, storage.StatusPageMonitor{
			PageID:    pageID,
			MonitorID: monitorID,
			SortOrder: sortOrder,
			GroupName: groupName,
		})
	}
	return result
}
