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
	GroupName   string
}

func (s *Server) handleWebStatusPageByID(w http.ResponseWriter, r *http.Request, pageID int64) {
	ctx := r.Context()

	sp, err := s.store.GetStatusPage(ctx, pageID)
	if err != nil || sp == nil || !sp.Enabled {
		http.NotFound(w, r)
		return
	}

	monitors, spms, err := s.store.ListStatusPageMonitorsWithStatus(ctx, sp.ID)
	if err != nil {
		s.logger.Error("web: status page monitors", "error", err)
		monitors = []*storage.Monitor{}
		spms = []storage.StatusPageMonitor{}
	}

	now := time.Now().UTC()
	from := now.AddDate(0, 0, -90)

	groupNameMap := make(map[int64]string, len(spms))
	for _, spm := range spms {
		groupNameMap[spm.MonitorID] = spm.GroupName
	}

	var monitorData []monitorWithUptime
	for _, m := range monitors {
		bars := s.buildDailyBars(ctx, m.ID, from, now)
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
			GroupName:   groupNameMap[m.ID],
		})
	}

	// Build grouped structure for template
	type monitorGroup struct {
		Name     string
		Monitors []monitorWithUptime
	}
	var groups []monitorGroup
	groupIdx := make(map[string]int)
	for _, md := range monitorData {
		gn := md.GroupName
		if idx, ok := groupIdx[gn]; ok {
			groups[idx].Monitors = append(groups[idx].Monitors, md)
		} else {
			groupIdx[gn] = len(groups)
			groups = append(groups, monitorGroup{Name: gn, Monitors: []monitorWithUptime{md}})
		}
	}

	overall := overallStatus(monitors)

	cfg := &storage.StatusPageConfig{
		ShowIncidents: sp.ShowIncidents,
		CustomCSS:     sp.CustomCSS,
	}
	incidents := s.publicIncidents(ctx, cfg, monitors, now)

	pd := pageData{
		Title:    sp.Title,
		BasePath: s.cfg.Server.BasePath,
		Data: map[string]interface{}{
			"Config":       sp,
			"Monitors":     monitorData,
			"Groups":       groups,
			"HasGroups":    len(groups) > 1 || (len(groups) == 1 && groups[0].Name != ""),
			"Overall":      overall,
			"Incidents":    incidents,
			"HasIncidents": len(incidents) > 0,
		},
	}
	s.renderStatusPage(w, pd)
}

func (s *Server) buildDailyBars(ctx context.Context, monitorID int64, from, now time.Time) []dailyBar {
	daily, err := s.store.GetDailyUptime(ctx, monitorID, from, now)
	if err != nil {
		s.logger.Error("web: status daily uptime", "monitor_id", monitorID, "error", err)
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
	return bars
}

// handleWebStatusPage handles legacy slug-based routing (backward compat)
func (s *Server) handleWebStatusPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Try multi-page first: look up by slug
	slug := s.getStatusSlug()
	sp, err := s.store.GetStatusPageBySlug(ctx, slug)
	if err == nil && sp != nil && sp.Enabled {
		s.handleWebStatusPageByID(w, r, sp.ID)
		return
	}

	// Fallback to legacy single-page
	cfg, err := s.store.GetStatusPageConfig(ctx)
	if err != nil || !cfg.Enabled {
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

	var monitorData []monitorWithUptime
	for _, m := range monitors {
		bars := s.buildDailyBars(ctx, m.ID, from, now)
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

	incResult, err := s.store.ListIncidents(ctx, 0, "", "", storage.Pagination{Page: 1, PerPage: 20})
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

func (s *Server) publicIncidentsForPage(ctx context.Context, sp *storage.StatusPage, monitors []*storage.Monitor, now time.Time) []*storage.Incident {
	if !sp.ShowIncidents {
		return []*storage.Incident{}
	}
	cfg := &storage.StatusPageConfig{ShowIncidents: true}
	return s.publicIncidents(ctx, cfg, monitors, now)
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
	"status-settings": true, "status-pages": true, "static": true, "api": true,
	"groups": true,
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
