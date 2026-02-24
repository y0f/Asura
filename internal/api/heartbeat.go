package api

import (
	"database/sql"
	"errors"
	"net/http"
)

func (h *Handler) HeartbeatPing(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	hb, err := h.store.GetHeartbeatByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "heartbeat not found")
			return
		}
		h.logger.Error("get heartbeat by token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get heartbeat")
		return
	}

	if err := h.store.UpdateHeartbeatPing(r.Context(), token); err != nil {
		h.logger.Error("update heartbeat ping", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to record ping")
		return
	}

	if hb.Status == "down" && h.pipeline != nil {
		mon, err := h.store.GetMonitor(r.Context(), hb.MonitorID)
		if err == nil && mon != nil {
			h.pipeline.ProcessHeartbeatRecovery(r.Context(), mon)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
