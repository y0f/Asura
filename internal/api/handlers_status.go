package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleGetStatusConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.GetStatusPageConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load status page config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleUpdateStatusConfig(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled       *bool   `json:"enabled"`
		Title         *string `json:"title"`
		Description   *string `json:"description"`
		ShowIncidents *bool   `json:"show_incidents"`
		CustomCSS     *string `json:"custom_css"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	cfg, err := s.store.GetStatusPageConfig(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load status page config")
		return
	}

	if input.Enabled != nil {
		cfg.Enabled = *input.Enabled
	}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			title = "Service Status"
		}
		if len(title) > 200 {
			title = title[:200]
		}
		cfg.Title = title
	}
	if input.Description != nil {
		desc := strings.TrimSpace(*input.Description)
		if len(desc) > 1000 {
			desc = desc[:1000]
		}
		cfg.Description = desc
	}
	if input.ShowIncidents != nil {
		cfg.ShowIncidents = *input.ShowIncidents
	}
	if input.CustomCSS != nil {
		cfg.CustomCSS = sanitizeCSS(*input.CustomCSS)
	}

	if err := s.store.UpsertStatusPageConfig(ctx, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save status page config")
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePublicStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cfg, err := s.store.GetStatusPageConfig(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load status page config")
		return
	}
	if !cfg.Enabled {
		writeError(w, http.StatusNotFound, "status page is not enabled")
		return
	}

	monitors, err := s.store.ListPublicMonitors(ctx)
	if err != nil {
		s.logger.Error("public status: list monitors", "error", err)
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
			s.logger.Error("public status: daily uptime", "monitor_id", m.ID, "error", err)
			daily = []*storage.DailyUptime{}
		}

		uptime, err := s.store.GetUptimePercent(ctx, m.ID, from, now)
		if err != nil {
			s.logger.Error("public status: uptime percent", "monitor_id", m.ID, "error", err)
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
	incidents := s.publicIncidents(ctx, cfg, monitors, now)

	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"page": map[string]string{
			"title":       cfg.Title,
			"description": cfg.Description,
		},
		"overall_status": overall,
		"monitors":       result,
		"incidents":      incidents,
	})
}
