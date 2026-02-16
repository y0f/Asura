package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/asura-monitor/asura/internal/storage"
)

func (s *Server) handleListMaintenance(w http.ResponseWriter, r *http.Request) {
	windows, err := s.store.ListMaintenanceWindows(r.Context())
	if err != nil {
		s.logger.Error("list maintenance", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list maintenance windows")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": windows})
}

func (s *Server) handleCreateMaintenance(w http.ResponseWriter, r *http.Request) {
	var mw storage.MaintenanceWindow
	if err := readJSON(r, &mw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if mw.MonitorIDs == nil {
		mw.MonitorIDs = []int64{}
	}

	if err := validateMaintenanceWindow(&mw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreateMaintenanceWindow(r.Context(), &mw); err != nil {
		s.logger.Error("create maintenance", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create maintenance window")
		return
	}

	s.audit(r, "create", "maintenance_window", mw.ID, "")
	writeJSON(w, http.StatusCreated, mw)
}

func (s *Server) handleUpdateMaintenance(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = s.store.GetMaintenanceWindow(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "maintenance window not found")
			return
		}
		s.logger.Error("get maintenance for update", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get maintenance window")
		return
	}

	var mw storage.MaintenanceWindow
	if err := readJSON(r, &mw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	mw.ID = id
	if mw.MonitorIDs == nil {
		mw.MonitorIDs = []int64{}
	}

	if err := validateMaintenanceWindow(&mw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.UpdateMaintenanceWindow(r.Context(), &mw); err != nil {
		s.logger.Error("update maintenance", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update maintenance window")
		return
	}

	s.audit(r, "update", "maintenance_window", mw.ID, "")
	writeJSON(w, http.StatusOK, mw)
}

func (s *Server) handleDeleteMaintenance(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = s.store.GetMaintenanceWindow(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "maintenance window not found")
			return
		}
		s.logger.Error("get maintenance for delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get maintenance window")
		return
	}

	if err := s.store.DeleteMaintenanceWindow(r.Context(), id); err != nil {
		s.logger.Error("delete maintenance", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete maintenance window")
		return
	}

	s.audit(r, "delete", "maintenance_window", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
