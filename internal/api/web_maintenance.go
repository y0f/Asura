package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asura-monitor/asura/internal/storage"
)

func (s *Server) handleWebMaintenance(w http.ResponseWriter, r *http.Request) {
	windows, err := s.store.ListMaintenanceWindows(r.Context())
	if err != nil {
		s.logger.Error("web: list maintenance", "error", err)
	}

	pd := s.newPageData(r, "Maintenance", "maintenance")
	pd.Data = windows
	s.render(w, "maintenance.html", pd)
}

func (s *Server) handleWebMaintenanceCreate(w http.ResponseWriter, r *http.Request) {
	mw := s.parseMaintenanceForm(r)
	if err := s.store.CreateMaintenanceWindow(r.Context(), mw); err != nil {
		s.logger.Error("web: create maintenance", "error", err)
		s.setFlash(w, "Failed to create maintenance window")
		http.Redirect(w, r, "/maintenance", http.StatusSeeOther)
		return
	}
	s.setFlash(w, "Maintenance window created")
	http.Redirect(w, r, "/maintenance", http.StatusSeeOther)
}

func (s *Server) handleWebMaintenanceUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	mw := s.parseMaintenanceForm(r)
	mw.ID = id
	if err := s.store.UpdateMaintenanceWindow(r.Context(), mw); err != nil {
		s.logger.Error("web: update maintenance", "error", err)
	}
	s.setFlash(w, "Maintenance window updated")
	http.Redirect(w, r, "/maintenance", http.StatusSeeOther)
}

func (s *Server) handleWebMaintenanceDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	if err := s.store.DeleteMaintenanceWindow(r.Context(), id); err != nil {
		s.logger.Error("web: delete maintenance", "error", err)
	}
	s.setFlash(w, "Maintenance window deleted")
	http.Redirect(w, r, "/maintenance", http.StatusSeeOther)
}

func (s *Server) parseMaintenanceForm(r *http.Request) *storage.MaintenanceWindow {
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
