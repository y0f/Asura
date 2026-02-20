package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleListStatusPages(w http.ResponseWriter, r *http.Request) {
	pages, err := s.store.ListStatusPages(r.Context())
	if err != nil {
		s.logger.Error("list status pages", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list status pages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": pages})
}

func (s *Server) handleGetStatusPage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sp, err := s.store.GetStatusPage(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "status page not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get status page")
		return
	}

	monitors, err := s.store.ListStatusPageMonitors(r.Context(), id)
	if err != nil {
		s.logger.Error("get status page monitors", "error", err)
		monitors = []storage.StatusPageMonitor{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status_page": sp,
		"monitors":    monitors,
	})
}

func (s *Server) handleCreateStatusPage(w http.ResponseWriter, r *http.Request) {
	var input struct {
		storage.StatusPage
		Monitors []storage.StatusPageMonitor `json:"monitors"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sp := &input.StatusPage
	if err := validateStatusPage(sp); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()

	// Check slug uniqueness
	existing, err := s.store.GetStatusPageBySlug(ctx, sp.Slug)
	if err == nil && existing != nil {
		writeError(w, http.StatusConflict, "slug already in use")
		return
	}

	if err := s.store.CreateStatusPage(ctx, sp); err != nil {
		s.logger.Error("create status page", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create status page")
		return
	}

	if len(input.Monitors) > 0 {
		for i := range input.Monitors {
			input.Monitors[i].PageID = sp.ID
		}
		if err := s.store.SetStatusPageMonitors(ctx, sp.ID, input.Monitors); err != nil {
			s.logger.Error("set status page monitors", "error", err)
		}
	}

	s.refreshStatusSlugs()
	s.audit(r, "create", "status_page", sp.ID, "")
	writeJSON(w, http.StatusCreated, sp)
}

func (s *Server) handleUpdateStatusPage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	_, err = s.store.GetStatusPage(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "status page not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get status page")
		return
	}

	var input struct {
		storage.StatusPage
		Monitors *[]storage.StatusPageMonitor `json:"monitors"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sp := &input.StatusPage
	sp.ID = id
	if err := validateStatusPage(sp); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check slug uniqueness (excluding self)
	existing, err := s.store.GetStatusPageBySlug(ctx, sp.Slug)
	if err == nil && existing != nil && existing.ID != id {
		writeError(w, http.StatusConflict, "slug already in use")
		return
	}

	if err := s.store.UpdateStatusPage(ctx, sp); err != nil {
		s.logger.Error("update status page", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update status page")
		return
	}

	if input.Monitors != nil {
		for i := range *input.Monitors {
			(*input.Monitors)[i].PageID = id
		}
		if err := s.store.SetStatusPageMonitors(ctx, id, *input.Monitors); err != nil {
			s.logger.Error("set status page monitors", "error", err)
		}
	}

	s.refreshStatusSlugs()
	s.audit(r, "update", "status_page", id, "")
	writeJSON(w, http.StatusOK, sp)
}

func (s *Server) handleDeleteStatusPage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	_, err = s.store.GetStatusPage(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "status page not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get status page")
		return
	}

	if err := s.store.DeleteStatusPage(ctx, id); err != nil {
		s.logger.Error("delete status page", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete status page")
		return
	}

	s.refreshStatusSlugs()
	s.audit(r, "delete", "status_page", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handlePublicStatusPage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	sp, err := s.store.GetStatusPage(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "status page not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get status page")
		return
	}

	if !sp.Enabled && !sp.APIEnabled {
		writeError(w, http.StatusNotFound, "status page is not enabled")
		return
	}

	monitors, _, err := s.store.ListStatusPageMonitorsWithStatus(ctx, sp.ID)
	if err != nil {
		s.logger.Error("public status page: list monitors", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load monitors")
		return
	}

	now := time.Now().UTC()
	from := now.AddDate(0, 0, -90)

	type safeMonitor struct {
		ID          int64                  `json:"id"`
		Name        string                 `json:"name"`
		Type        string                 `json:"type"`
		Status      string                 `json:"status"`
		Uptime90d   float64                `json:"uptime_90d"`
		DailyUptime []*storage.DailyUptime `json:"daily_uptime"`
	}

	result := make([]safeMonitor, 0, len(monitors))
	for _, m := range monitors {
		daily, err := s.store.GetDailyUptime(ctx, m.ID, from, now)
		if err != nil {
			daily = []*storage.DailyUptime{}
		}
		uptime, err := s.store.GetUptimePercent(ctx, m.ID, from, now)
		if err != nil {
			uptime = 100
		}
		result = append(result, safeMonitor{
			ID:          m.ID,
			Name:        m.Name,
			Type:        m.Type,
			Status:      m.Status,
			Uptime90d:   uptime,
			DailyUptime: daily,
		})
	}

	overall := overallStatus(monitors)
	incidents := s.publicIncidentsForPage(ctx, sp, monitors, now)

	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"page": map[string]string{
			"title":       sp.Title,
			"description": sp.Description,
		},
		"overall_status": overall,
		"monitors":       result,
		"incidents":      incidents,
	})
}
