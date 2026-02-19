package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleWebStatusPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cfg, err := s.store.GetStatusPageConfig(ctx)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !cfg.Enabled {
		http.NotFound(w, r)
		return
	}

	monitors, err := s.store.ListPublicMonitors(ctx)
	if err != nil {
		s.logger.Error("web: status page monitors", "error", err)
		monitors = []*storage.Monitor{}
	}

	now := time.Now().UTC()
	from := now.AddDate(0, 0, -90)

	type dailyBar struct {
		Date      string
		UptimePct float64
		HasData   bool
		Label     string
	}

	type monitorWithUptime struct {
		Monitor     *storage.Monitor
		DailyBars   []dailyBar
		Uptime90d   float64
		UptimeLabel string
	}

	var monitorData []monitorWithUptime
	for _, m := range monitors {
		daily, err := s.store.GetDailyUptime(ctx, m.ID, from, now)
		if err != nil {
			s.logger.Error("web: status daily uptime", "monitor_id", m.ID, "error", err)
		}

		dayMap := make(map[string]*storage.DailyUptime)
		for _, d := range daily {
			dayMap[d.Date] = d
		}

		bars := make([]dailyBar, 0, 90)
		for i := 89; i >= 0; i-- {
			day := now.AddDate(0, 0, -i)
			dateStr := day.Format("2006-01-02")
			label := day.Format("Jan 2, 2006")
			if d, ok := dayMap[dateStr]; ok {
				bars = append(bars, dailyBar{
					Date:      dateStr,
					UptimePct: d.UptimePct,
					HasData:   true,
					Label:     label,
				})
			} else {
				bars = append(bars, dailyBar{
					Date:    dateStr,
					HasData: false,
					Label:   label,
				})
			}
		}

		uptime, err := s.store.GetUptimePercent(ctx, m.ID, from, now)
		if err != nil {
			s.logger.Error("web: status uptime percent", "monitor_id", m.ID, "error", err)
			uptime = 100
		}

		monitorData = append(monitorData, monitorWithUptime{
			Monitor:     m,
			DailyBars:   bars,
			Uptime90d:   uptime,
			UptimeLabel: formatPct(uptime),
		})
	}

	overall := overallStatus(monitors)
	incidents := s.publicIncidents(ctx, cfg, monitors, now)

	pd := pageData{
		Title:    cfg.Title,
		BasePath: s.cfg.Server.BasePath,
		Data: map[string]interface{}{
			"Config":       cfg,
			"Monitors":     monitorData,
			"Overall":      overall,
			"Incidents":    incidents,
			"HasIncidents": len(incidents) > 0,
		},
	}
	s.renderStatusPage(w, pd)
}

func (s *Server) handleWebStatusSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.GetStatusPageConfig(r.Context())
	if err != nil {
		s.logger.Error("web: get status config", "error", err)
		cfg = &storage.StatusPageConfig{Title: "Service Status"}
	}

	pd := s.newPageData(r, "Status Page Settings", "status-settings")
	pd.Data = cfg
	s.render(w, "status_settings.html", pd)
}

func (s *Server) handleWebStatusSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = "Service Status"
	}
	if len(title) > 200 {
		title = title[:200]
	}

	desc := strings.TrimSpace(r.FormValue("description"))
	if len(desc) > 1000 {
		desc = desc[:1000]
	}

	slug := validateSlug(r.FormValue("slug"))

	cfg := &storage.StatusPageConfig{
		Enabled:       r.FormValue("enabled") == "on",
		Title:         title,
		Description:   desc,
		ShowIncidents: r.FormValue("show_incidents") == "on",
		CustomCSS:     sanitizeCSS(r.FormValue("custom_css")),
		Slug:          slug,
	}

	if err := s.store.UpsertStatusPageConfig(r.Context(), cfg); err != nil {
		s.logger.Error("web: update status config", "error", err)
		s.setFlash(w, "Failed to save settings")
		s.redirect(w, r, "/status-settings")
		return
	}

	s.setStatusSlug(slug)
	s.setFlash(w, "Status page settings saved")
	s.redirect(w, r, "/status-settings")
}

func (s *Server) renderStatusPage(w http.ResponseWriter, data pageData) {
	t, ok := s.templates["status_page.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
	if err := t.ExecuteTemplate(w, "status_page", data); err != nil {
		s.logger.Error("template render", "template", "status_page.html", "error", err)
	}
}

func overallStatus(monitors []*storage.Monitor) string {
	overall := "operational"
	for _, m := range monitors {
		if m.Status == "down" {
			return "major_outage"
		}
		if m.Status == "degraded" {
			overall = "degraded"
		}
	}
	return overall
}

func (s *Server) publicIncidents(ctx context.Context, cfg *storage.StatusPageConfig, monitors []*storage.Monitor, now time.Time) []*storage.Incident {
	if !cfg.ShowIncidents {
		return []*storage.Incident{}
	}

	publicIDs := make(map[int64]bool, len(monitors))
	for _, m := range monitors {
		publicIDs[m.ID] = true
	}

	incResult, err := s.store.ListIncidents(ctx, 0, "", storage.Pagination{Page: 1, PerPage: 20})
	if err != nil || incResult == nil {
		return []*storage.Incident{}
	}

	all, ok := incResult.Data.([]*storage.Incident)
	if !ok {
		return []*storage.Incident{}
	}

	cutoff := now.AddDate(0, 0, -7)
	var filtered []*storage.Incident
	for _, inc := range all {
		if !publicIDs[inc.MonitorID] {
			continue
		}
		if inc.Status == "resolved" && inc.ResolvedAt != nil && inc.ResolvedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, inc)
		if len(filtered) >= 10 {
			break
		}
	}
	if filtered == nil {
		filtered = []*storage.Incident{}
	}
	return filtered
}

func formatPct(pct float64) string {
	if pct >= 99.995 {
		return "100%"
	}
	return fmt.Sprintf("%.2f%%", pct)
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

var reservedSlugs = map[string]bool{
	"login": true, "logout": true, "monitors": true, "incidents": true,
	"notifications": true, "maintenance": true, "logs": true,
	"status-settings": true, "static": true, "api": true,
}

func validateSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return "status"
	}
	if len(slug) > 50 {
		slug = slug[:50]
	}
	if !slugPattern.MatchString(slug) {
		return "status"
	}
	if reservedSlugs[slug] {
		return "status"
	}
	return slug
}

var unsafeCSSPattern = regexp.MustCompile(`(?i)(javascript|vbscript|expression|behavior|@import|@charset|url\s*\(|-moz-binding)`)

func sanitizeCSS(css string) string {
	if len(css) > 10000 {
		css = css[:10000]
	}
	css = strings.ReplaceAll(css, "<", "")
	css = strings.ReplaceAll(css, ">", "")
	css = strings.ReplaceAll(css, "\\", "")
	css = unsafeCSSPattern.ReplaceAllString(css, "")
	return css
}
