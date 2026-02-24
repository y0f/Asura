package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/y0f/asura/internal/httputil"
	"github.com/y0f/asura/internal/storage"
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

func (h *Handler) StatusPageByID(w http.ResponseWriter, r *http.Request, pageID int64) {
	ctx := r.Context()

	sp, err := h.store.GetStatusPage(ctx, pageID)
	if err != nil || sp == nil || !sp.Enabled {
		http.NotFound(w, r)
		return
	}

	monitors, spms, err := h.store.ListStatusPageMonitorsWithStatus(ctx, sp.ID)
	if err != nil {
		h.logger.Error("web: status page monitors", "error", err)
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
		bars := h.buildDailyBars(ctx, m.ID, from, now)
		uptime, err := h.store.GetUptimePercent(ctx, m.ID, from, now)
		if err != nil {
			h.logger.Error("web: status uptime percent", "monitor_id", m.ID, "error", err)
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

	overall := httputil.OverallStatus(monitors)

	incidents := httputil.PublicIncidentsForPage(ctx, h.store, sp, monitors, now)

	pd := pageData{
		Title:    sp.Title,
		BasePath: h.cfg.Server.BasePath,
		Data: map[string]any{
			"Config":       sp,
			"Monitors":     monitorData,
			"Groups":       groups,
			"HasGroups":    len(groups) > 1 || (len(groups) == 1 && groups[0].Name != ""),
			"Overall":      overall,
			"Incidents":    incidents,
			"HasIncidents": len(incidents) > 0,
		},
	}
	h.renderStatusPage(w, pd)
}

func (h *Handler) buildDailyBars(ctx context.Context, monitorID int64, from, now time.Time) []dailyBar {
	daily, err := h.store.GetDailyUptime(ctx, monitorID, from, now)
	if err != nil {
		h.logger.Error("web: status daily uptime", "monitor_id", monitorID, "error", err)
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

func (h *Handler) renderStatusPage(w http.ResponseWriter, data pageData) {
	t, ok := h.templates["status/page.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
	if err := t.ExecuteTemplate(w, "status_page", data); err != nil {
		h.logger.Error("template render", "template", "status/page.html", "error", err)
	}
}

func formatPct(pct float64) string {
	if pct >= 99.995 {
		return "100%"
	}
	return fmt.Sprintf("%.2f%%", pct)
}
