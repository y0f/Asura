package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListMonitorGroups(r.Context())
	if err != nil {
		s.logger.Error("list groups", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": groups})
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var g storage.MonitorGroup
	if err := readJSON(r, &g); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := validateMonitorGroup(&g); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreateMonitorGroup(r.Context(), &g); err != nil {
		s.logger.Error("create group", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create group")
		return
	}

	s.audit(r, "create", "monitor_group", g.ID, "")
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = s.store.GetMonitorGroup(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		s.logger.Error("get group for update", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get group")
		return
	}

	var g storage.MonitorGroup
	if err := readJSON(r, &g); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	g.ID = id

	if err := validateMonitorGroup(&g); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.UpdateMonitorGroup(r.Context(), &g); err != nil {
		s.logger.Error("update group", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update group")
		return
	}

	s.audit(r, "update", "monitor_group", g.ID, "")
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = s.store.GetMonitorGroup(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		s.logger.Error("get group for delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get group")
		return
	}

	if err := s.store.DeleteMonitorGroup(r.Context(), id); err != nil {
		s.logger.Error("delete group", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete group")
		return
	}

	s.audit(r, "delete", "monitor_group", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
