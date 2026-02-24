package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/y0f/Asura/internal/storage"
)

func (s *Server) handleListProxies(w http.ResponseWriter, r *http.Request) {
	proxies, err := s.store.ListProxies(r.Context())
	if err != nil {
		s.logger.Error("list proxies", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list proxies")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": proxies})
}

func (s *Server) handleGetProxy(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.store.GetProxy(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "proxy not found")
			return
		}
		s.logger.Error("get proxy", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get proxy")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCreateProxy(w http.ResponseWriter, r *http.Request) {
	var p storage.Proxy
	if err := readJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := validateProxy(&p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.CreateProxy(r.Context(), &p); err != nil {
		s.logger.Error("create proxy", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create proxy")
		return
	}

	s.audit(r, "create", "proxy", p.ID, "")
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdateProxy(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = s.store.GetProxy(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "proxy not found")
			return
		}
		s.logger.Error("get proxy for update", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get proxy")
		return
	}

	var p storage.Proxy
	if err := readJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.ID = id

	if err := validateProxy(&p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.UpdateProxy(r.Context(), &p); err != nil {
		s.logger.Error("update proxy", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update proxy")
		return
	}

	updated, _ := s.store.GetProxy(r.Context(), id)
	if updated == nil {
		updated = &p
	}

	s.audit(r, "update", "proxy", p.ID, "")
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteProxy(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = s.store.GetProxy(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "proxy not found")
			return
		}
		s.logger.Error("get proxy for delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get proxy")
		return
	}

	if err := s.store.DeleteProxy(r.Context(), id); err != nil {
		s.logger.Error("delete proxy", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete proxy")
		return
	}

	s.audit(r, "delete", "proxy", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
