package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/y0f/asura/internal/httputil"
	"github.com/y0f/asura/internal/storage"
	"github.com/y0f/asura/internal/validate"
)

func (h *Handler) Maintenance(w http.ResponseWriter, r *http.Request) {
	windows, err := h.store.ListMaintenanceWindows(r.Context())
	if err != nil {
		h.logger.Error("web: list maintenance", "error", err)
	}

	pd := h.newPageData(r, "Maintenance", "maintenance")
	pd.Data = windows
	h.render(w, "maintenance/list.html", pd)
}

func (h *Handler) MaintenanceCreate(w http.ResponseWriter, r *http.Request) {
	mw := h.parseMaintenanceForm(r)
	if err := validate.ValidateMaintenanceWindow(mw); err != nil {
		h.setFlash(w, err.Error())
		h.redirect(w, r, "/maintenance")
		return
	}
	if err := h.store.CreateMaintenanceWindow(r.Context(), mw); err != nil {
		h.logger.Error("web: create maintenance", "error", err)
		h.setFlash(w, "Failed to create maintenance window")
		h.redirect(w, r, "/maintenance")
		return
	}
	h.setFlash(w, "Maintenance window created")
	h.redirect(w, r, "/maintenance")
}

func (h *Handler) MaintenanceUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := httputil.ParseID(r)
	mw := h.parseMaintenanceForm(r)
	mw.ID = id
	if err := validate.ValidateMaintenanceWindow(mw); err != nil {
		h.setFlash(w, err.Error())
		h.redirect(w, r, "/maintenance")
		return
	}
	if err := h.store.UpdateMaintenanceWindow(r.Context(), mw); err != nil {
		h.logger.Error("web: update maintenance", "error", err)
	}
	h.setFlash(w, "Maintenance window updated")
	h.redirect(w, r, "/maintenance")
}

func (h *Handler) MaintenanceDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := httputil.ParseID(r)
	if err := h.store.DeleteMaintenanceWindow(r.Context(), id); err != nil {
		h.logger.Error("web: delete maintenance", "error", err)
	}
	h.setFlash(w, "Maintenance window deleted")
	h.redirect(w, r, "/maintenance")
}

func (h *Handler) parseMaintenanceForm(r *http.Request) *storage.MaintenanceWindow {
	r.ParseForm()

	startTime, _ := time.Parse("2006-01-02T15:04", r.FormValue("start_time"))
	endTime, _ := time.Parse("2006-01-02T15:04", r.FormValue("end_time"))

	mw := &storage.MaintenanceWindow{
		Name:      r.FormValue("name"),
		StartTime: startTime,
		EndTime:   endTime,
		Recurring: r.FormValue("recurring"),
	}

	if ids := r.FormValue("monitor_ids"); ids != "" {
		for _, idStr := range strings.Split(ids, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64); err == nil {
				mw.MonitorIDs = append(mw.MonitorIDs, id)
			}
		}
	}

	return mw
}
