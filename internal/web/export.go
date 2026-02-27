package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/y0f/asura/internal/api"
	"github.com/y0f/asura/internal/web/views"
)

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	lp := h.newLayoutParams(r, "Settings", "settings")
	h.renderComponent(w, r, views.SettingsPage(views.SettingsParams{
		LayoutParams: lp,
	}))
}

func (h *Handler) ExportConfig(w http.ResponseWriter, r *http.Request) {
	redact := r.URL.Query().Get("redact_secrets") == "true"

	data, err := api.BuildExportData(r.Context(), h.store, redact)
	if err != nil {
		h.setFlash(w, "Failed to build export data")
		h.redirect(w, r, "/settings")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="asura-export.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}

func (h *Handler) ImportConfig(w http.ResponseWriter, r *http.Request) {
	mode := r.FormValue("mode")
	if mode == "" {
		mode = "merge"
	}
	if mode != "merge" && mode != "replace" {
		h.setFlash(w, "Invalid import mode")
		h.redirect(w, r, "/settings")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		h.setFlash(w, "No file uploaded")
		h.redirect(w, r, "/settings")
		return
	}
	defer file.Close()

	body, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		h.setFlash(w, "Failed to read file")
		h.redirect(w, r, "/settings")
		return
	}

	var data api.ExportData
	if err := json.Unmarshal(body, &data); err != nil {
		h.setFlash(w, "Invalid JSON file")
		h.redirect(w, r, "/settings")
		return
	}
	if data.Version != 1 {
		h.setFlash(w, "Unsupported export version")
		h.redirect(w, r, "/settings")
		return
	}

	stats := api.RunImport(r.Context(), h.store, &data, mode)

	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}
	if h.OnStatusPageChange != nil {
		h.OnStatusPageChange()
	}

	h.setFlash(w, fmt.Sprintf("Imported: %d monitors, %d channels, %d groups, %d proxies, %d maintenance, %d status pages (%d skipped, %d errors)",
		stats.Monitors, stats.Channels, stats.Groups, stats.Proxies, stats.Maintenance, stats.StatusPages, stats.Skipped, stats.Errors))
	h.redirect(w, r, "/settings")
}
