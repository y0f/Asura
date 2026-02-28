package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/y0f/asura/internal/incident"
	"github.com/y0f/asura/internal/storage"
	"github.com/y0f/asura/internal/validate"
	"github.com/y0f/asura/internal/web/views"
)

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	const perPage = 10

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	typeFilter := r.URL.Query().Get("type")
	if !validate.ValidMonitorTypes[typeFilter] {
		typeFilter = ""
	}

	allMonitors := h.loadAllMonitors(ctx)
	up, down, degraded, paused := countMonitorStats(allMonitors)
	filtered := filterMonitorsByType(allMonitors, typeFilter)
	displayMonitors, page, totalPages := paginate(filtered, page, perPage)

	incidentList := h.loadOpenIncidents(ctx)

	responseTimes, _ := h.store.GetLatestResponseTimes(ctx)
	if responseTimes == nil {
		responseTimes = make(map[int64]int64)
	}

	monitorIDs := make([]int64, len(displayMonitors))
	for i, m := range displayMonitors {
		monitorIDs[i] = m.ID
	}
	sparklines, _ := h.store.GetMonitorSparklines(ctx, monitorIDs, 20)
	if sparklines == nil {
		sparklines = make(map[int64][]*storage.SparklinePoint)
	}

	now := time.Now().UTC()
	requests24h, visitors24h := h.loadRequestStats(ctx, now)

	lp := h.newLayoutParams(r, "Dashboard", "dashboard")
	h.renderComponent(w, r, views.DashboardPage(views.DashboardParams{
		LayoutParams:  lp,
		Monitors:      displayMonitors,
		Incidents:     incidentList,
		ResponseTimes: responseTimes,
		Sparklines:    sparklines,
		Total:         len(allMonitors),
		Up:            up,
		Down:          down,
		Degraded:      degraded,
		Paused:        paused,
		OpenIncidents: len(incidentList),
		Requests24h:   requests24h,
		Visitors24h:   visitors24h,
		Page:          page,
		TotalPages:    totalPages,
		Filter:        typeFilter,
	}))
}

func (h *Handler) loadAllMonitors(ctx context.Context) []*storage.Monitor {
	result, err := h.store.ListMonitors(ctx, storage.MonitorListFilter{}, storage.Pagination{Page: 1, PerPage: 1000})
	if err != nil {
		h.logger.Error("web: list monitors", "error", err)
		return nil
	}
	if result == nil {
		return nil
	}
	ml, _ := result.Data.([]*storage.Monitor)
	return ml
}

func (h *Handler) loadOpenIncidents(ctx context.Context) []*storage.Incident {
	result, err := h.store.ListIncidents(ctx, 0, incident.StatusOpen, "", storage.Pagination{Page: 1, PerPage: 10})
	if err != nil {
		h.logger.Error("web: list incidents", "error", err)
		return nil
	}
	if result == nil {
		return nil
	}
	il, _ := result.Data.([]*storage.Incident)
	return il
}

func (h *Handler) loadRequestStats(ctx context.Context, now time.Time) (requests, visitors int64) {
	stats, err := h.store.GetRequestLogStats(ctx, now.Add(-24*time.Hour), now)
	if err != nil {
		h.logger.Error("web: request log stats", "error", err)
		return 0, 0
	}
	if stats == nil {
		return 0, 0
	}
	return stats.TotalRequests, stats.UniqueVisitors
}

func paginate[T any](items []T, page, perPage int) ([]T, int, int) {
	total := len(items)
	totalPages := (total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * perPage
	end := start + perPage
	if end > total {
		end = total
	}
	return items[start:end], page, totalPages
}

func countMonitorStats(monitors []*storage.Monitor) (up, down, degraded, paused int) {
	for _, m := range monitors {
		if !m.Enabled {
			paused++
			continue
		}
		switch m.Status {
		case "down":
			down++
		case "degraded":
			degraded++
		default:
			up++
		}
	}
	return
}

func filterMonitorsByType(monitors []*storage.Monitor, t string) []*storage.Monitor {
	if t == "" {
		return monitors
	}
	out := make([]*storage.Monitor, 0, len(monitors))
	for _, m := range monitors {
		if m.Type == t {
			out = append(out, m)
		}
	}
	return out
}
