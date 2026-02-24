package api

import (
	"net/http"
	"time"
)

var startTime = time.Now()

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"uptime": time.Since(startTime).String(),
	})
}
