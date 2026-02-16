package api

import (
	"database/sql"
	"errors"
	"net/http"
)

func (s *Server) handleHeartbeatPing(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	hb, err := s.store.GetHeartbeatByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "heartbeat not found")
			return
		}
		s.logger.Error("get heartbeat by token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get heartbeat")
		return
	}

	if err := s.store.UpdateHeartbeatPing(r.Context(), token); err != nil {
		s.logger.Error("update heartbeat ping", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to record ping")
		return
	}

	// If heartbeat was down, update monitor status to up
	if hb.Status == "down" && s.pipeline != nil {
		mon, err := s.store.GetMonitor(r.Context(), hb.MonitorID)
		if err == nil && mon != nil {
			s.pipeline.ProcessHeartbeatRecovery(r.Context(), mon)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
