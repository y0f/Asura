package api

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/assertion"
	"github.com/y0f/Asura/internal/storage"
)

type headerPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type monitorFormData struct {
	Monitor         *storage.Monitor
	HTTP            storage.HTTPSettings
	TCP             storage.TCPSettings
	DNS             storage.DNSSettings
	TLS             storage.TLSSettings
	WS              storage.WebSocketSettings
	Cmd             storage.CommandSettings
	FollowRedirects bool
	MaxRedirects    int
	HeadersJSON     template.JS
	WsHeadersJSON   template.JS
	AssertionsJSON  template.JS
	SettingsJSON    string
	AssertionsRaw   string
}

func monitorToFormData(mon *storage.Monitor) *monitorFormData {
	fd := &monitorFormData{Monitor: mon}
	if mon == nil {
		fd.Monitor = &storage.Monitor{}
		fd.FollowRedirects = true
		fd.MaxRedirects = 10
		fd.HTTP.AuthMethod = "none"
		fd.HTTP.BodyEncoding = "json"
		fd.HeadersJSON = "[]"
		fd.WsHeadersJSON = "[]"
		fd.AssertionsJSON = "[]"
		fd.SettingsJSON = "{}"
		fd.AssertionsRaw = "[]"
		return fd
	}

	if len(mon.Settings) > 0 {
		fd.SettingsJSON = string(mon.Settings)
	} else {
		fd.SettingsJSON = "{}"
	}
	if len(mon.Assertions) > 0 {
		fd.AssertionsRaw = string(mon.Assertions)
	} else {
		fd.AssertionsRaw = "[]"
	}

	switch mon.Type {
	case "http":
		json.Unmarshal(mon.Settings, &fd.HTTP)
	case "tcp":
		json.Unmarshal(mon.Settings, &fd.TCP)
	case "dns":
		json.Unmarshal(mon.Settings, &fd.DNS)
	case "tls":
		json.Unmarshal(mon.Settings, &fd.TLS)
	case "websocket":
		json.Unmarshal(mon.Settings, &fd.WS)
	case "command":
		json.Unmarshal(mon.Settings, &fd.Cmd)
	}

	fd.FollowRedirects = fd.HTTP.FollowRedirects == nil || *fd.HTTP.FollowRedirects
	fd.MaxRedirects = fd.HTTP.MaxRedirects
	if fd.MaxRedirects == 0 && fd.FollowRedirects {
		fd.MaxRedirects = 10
	}
	fd.HTTP.AuthMethod = inferHTTPAuthMethod(fd.HTTP)
	if fd.HTTP.BodyEncoding == "" {
		fd.HTTP.BodyEncoding = "json"
	}
	fd.HeadersJSON = headersToJSON(fd.HTTP.Headers)
	fd.WsHeadersJSON = headersToJSON(fd.WS.Headers)
	fd.AssertionsJSON = assertionsToJSON(mon.Assertions)
	return fd
}

func inferHTTPAuthMethod(h storage.HTTPSettings) string {
	if h.AuthMethod != "" {
		return h.AuthMethod
	}
	if h.BasicAuthUser != "" {
		return "basic"
	}
	if h.BearerToken != "" {
		return "bearer"
	}
	return "none"
}

func headersToJSON(headers map[string]string) template.JS {
	if len(headers) == 0 {
		return "[]"
	}
	pairs := make([]headerPair, 0, len(headers))
	for k, v := range headers {
		pairs = append(pairs, headerPair{Key: k, Value: v})
	}
	b, _ := json.Marshal(pairs)
	return template.JS(b)
}

func assertionsToJSON(raw json.RawMessage) template.JS {
	if len(raw) == 0 {
		return "[]"
	}
	var assertions []assertion.Assertion
	if err := json.Unmarshal(raw, &assertions); err != nil {
		return "[]"
	}
	b, _ := json.Marshal(assertions)
	return template.JS(b)
}

func assembleSettings(r *http.Request, monType string) json.RawMessage {
	switch monType {
	case "http":
		b, _ := json.Marshal(assembleHTTPSettings(r))
		return b
	case "tcp":
		s := storage.TCPSettings{
			SendData:   r.FormValue("settings_send_data"),
			ExpectData: r.FormValue("settings_expect_data"),
		}
		b, _ := json.Marshal(s)
		return b
	case "dns":
		s := storage.DNSSettings{
			RecordType: r.FormValue("settings_record_type"),
			Server:     r.FormValue("settings_dns_server"),
		}
		b, _ := json.Marshal(s)
		return b
	case "tls":
		s := storage.TLSSettings{}
		if v := r.FormValue("settings_warn_days_before"); v != "" {
			s.WarnDaysBefore, _ = strconv.Atoi(v)
		}
		b, _ := json.Marshal(s)
		return b
	case "websocket":
		s := storage.WebSocketSettings{
			SendMessage: r.FormValue("settings_send_message"),
			ExpectReply: r.FormValue("settings_expect_reply"),
			Headers:     assembleHeaders(r, "settings_ws_header_key", "settings_ws_header_value"),
		}
		b, _ := json.Marshal(s)
		return b
	case "command":
		s := storage.CommandSettings{
			Command: r.FormValue("settings_command"),
		}
		if argsStr := strings.TrimSpace(r.FormValue("settings_args")); argsStr != "" {
			for _, a := range strings.Split(argsStr, ",") {
				if trimmed := strings.TrimSpace(a); trimmed != "" {
					s.Args = append(s.Args, trimmed)
				}
			}
		}
		b, _ := json.Marshal(s)
		return b
	default:
		return nil
	}
}

func assembleHTTPSettings(r *http.Request) storage.HTTPSettings {
	s := storage.HTTPSettings{
		Method:       r.FormValue("settings_method"),
		Body:         r.FormValue("settings_body"),
		BodyEncoding: r.FormValue("settings_body_encoding"),
		AuthMethod:   r.FormValue("settings_auth_method"),
	}
	if v := r.FormValue("settings_expected_status"); v != "" {
		s.ExpectedStatus, _ = strconv.Atoi(v)
	}
	if v := r.FormValue("settings_max_redirects"); v != "" {
		s.MaxRedirects, _ = strconv.Atoi(v)
		if s.MaxRedirects == 0 {
			f := false
			s.FollowRedirects = &f
		}
	}
	s.SkipTLSVerify = r.FormValue("settings_skip_tls_verify") == "on"
	s.CacheBuster = r.FormValue("settings_cache_buster") == "on"
	s.Headers = assembleHeaders(r, "settings_header_key", "settings_header_value")
	switch s.AuthMethod {
	case "basic":
		s.BasicAuthUser = r.FormValue("settings_basic_auth_user")
		s.BasicAuthPass = r.FormValue("settings_basic_auth_pass")
	case "bearer":
		s.BearerToken = r.FormValue("settings_bearer_token")
	}
	return s
}

func assembleHeaders(r *http.Request, keyField, valueField string) map[string]string {
	keys := r.Form[keyField+"[]"]
	values := r.Form[valueField+"[]"]
	if len(keys) == 0 {
		keys = r.Form[keyField]
		values = r.Form[valueField]
	}
	headers := make(map[string]string)
	for i, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || i >= len(values) {
			continue
		}
		headers[k] = values[i]
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func assembleAssertions(r *http.Request) json.RawMessage {
	countStr := r.FormValue("assertion_count")
	count, _ := strconv.Atoi(countStr)
	if count == 0 {
		return nil
	}
	if count > 50 {
		count = 50
	}

	var assertions []assertion.Assertion
	for i := 0; i < count; i++ {
		idx := strconv.Itoa(i)
		aType := r.FormValue("assertion_type_" + idx)
		if aType == "" {
			continue
		}
		a := assertion.Assertion{
			Type:     aType,
			Operator: r.FormValue("assertion_operator_" + idx),
			Target:   r.FormValue("assertion_target_" + idx),
			Value:    r.FormValue("assertion_value_" + idx),
			Degraded: r.FormValue("assertion_degraded_"+idx) == "on",
		}
		assertions = append(assertions, a)
	}

	if len(assertions) == 0 {
		return nil
	}
	b, _ := json.Marshal(assertions)
	return b
}

func (s *Server) handleWebMonitors(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	typeFilter := r.URL.Query().Get("type")
	if !validMonitorTypes[typeFilter] {
		typeFilter = ""
	}

	f := storage.MonitorListFilter{Type: typeFilter, Search: q}
	result, err := s.store.ListMonitors(r.Context(), f, p)
	if err != nil {
		s.logger.Error("web: list monitors", "error", err)
	}

	pd := s.newPageData(r, "Monitors", "monitors")
	pd.Data = map[string]interface{}{
		"Result": result,
		"Search": q,
		"Type":   typeFilter,
	}
	s.render(w, "monitors.html", pd)
}

func (s *Server) handleWebMonitorDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.redirect(w, r, "/monitors")
		return
	}

	ctx := r.Context()
	mon, err := s.store.GetMonitor(ctx, id)
	if err != nil {
		s.redirect(w, r, "/monitors")
		return
	}

	now := time.Now().UTC()
	checks, _ := s.store.ListCheckResults(ctx, id, storage.Pagination{Page: 1, PerPage: 50})
	if checks == nil {
		checks = &storage.PaginatedResult{}
	}
	changes, _ := s.store.ListContentChanges(ctx, id, storage.Pagination{Page: 1, PerPage: 10})
	if changes == nil {
		changes = &storage.PaginatedResult{}
	}

	uptime24h, _ := s.store.GetUptimePercent(ctx, id, now.Add(-24*time.Hour), now)
	uptime7d, _ := s.store.GetUptimePercent(ctx, id, now.Add(-7*24*time.Hour), now)
	uptime30d, _ := s.store.GetUptimePercent(ctx, id, now.Add(-30*24*time.Hour), now)
	p50, p95, p99, _ := s.store.GetResponseTimePercentiles(ctx, id, now.Add(-24*time.Hour), now)
	totalChecks, upChecks, downChecks, _, _ := s.store.GetCheckCounts(ctx, id, now.Add(-24*time.Hour), now)
	latestCheck, _ := s.store.GetLatestCheckResult(ctx, id)
	openIncident, _ := s.store.GetOpenIncident(ctx, id)

	pd := s.newPageData(r, mon.Name, "monitors")
	pd.Data = map[string]interface{}{
		"Monitor":      mon,
		"Checks":       checks,
		"Changes":      changes,
		"Uptime24h":    uptime24h,
		"Uptime7d":     uptime7d,
		"Uptime30d":    uptime30d,
		"P50":          p50,
		"P95":          p95,
		"P99":          p99,
		"TotalChecks":  totalChecks,
		"UpChecks":     upChecks,
		"DownChecks":   downChecks,
		"LatestCheck":  latestCheck,
		"OpenIncident": openIncident,
	}
	s.render(w, "monitor_detail.html", pd)
}

func (s *Server) handleWebMonitorForm(w http.ResponseWriter, r *http.Request) {
	pd := s.newPageData(r, "New Monitor", "monitors")

	idStr := r.PathValue("id")
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			s.redirect(w, r, "/monitors")
			return
		}
		mon, err := s.store.GetMonitor(r.Context(), id)
		if err != nil {
			s.redirect(w, r, "/monitors")
			return
		}
		pd.Title = "Edit " + mon.Name
		pd.Data = monitorToFormData(mon)
	} else {
		pd.Data = monitorToFormData(nil)
	}

	s.render(w, "monitor_form.html", pd)
}

func (s *Server) handleWebMonitorCreate(w http.ResponseWriter, r *http.Request) {
	mon := s.parseMonitorForm(r)

	s.applyMonitorDefaults(mon)

	if err := validateMonitor(mon); err != nil {
		pd := s.newPageData(r, "New Monitor", "monitors")
		pd.Error = err.Error()
		pd.Data = monitorToFormData(mon)
		s.render(w, "monitor_form.html", pd)
		return
	}

	if err := s.store.CreateMonitor(r.Context(), mon); err != nil {
		s.logger.Error("web: create monitor", "error", err)
		pd := s.newPageData(r, "New Monitor", "monitors")
		pd.Error = "Failed to create monitor"
		pd.Data = monitorToFormData(mon)
		s.render(w, "monitor_form.html", pd)
		return
	}

	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}

	s.setFlash(w, "Monitor created")
	s.redirect(w, r, "/monitors")
}

func (s *Server) handleWebMonitorUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.redirect(w, r, "/monitors")
		return
	}

	mon := s.parseMonitorForm(r)
	mon.ID = id

	if err := validateMonitor(mon); err != nil {
		pd := s.newPageData(r, "Edit Monitor", "monitors")
		pd.Error = err.Error()
		pd.Data = monitorToFormData(mon)
		s.render(w, "monitor_form.html", pd)
		return
	}

	if err := s.store.UpdateMonitor(r.Context(), mon); err != nil {
		s.logger.Error("web: update monitor", "error", err)
		pd := s.newPageData(r, "Edit Monitor", "monitors")
		pd.Error = "Failed to update monitor"
		pd.Data = monitorToFormData(mon)
		s.render(w, "monitor_form.html", pd)
		return
	}

	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}

	s.setFlash(w, "Monitor updated")
	s.redirect(w, r, "/monitors/"+strconv.FormatInt(id, 10))
}

func (s *Server) handleWebMonitorDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.redirect(w, r, "/monitors")
		return
	}

	if err := s.store.DeleteMonitor(r.Context(), id); err != nil {
		s.logger.Error("web: delete monitor", "error", err)
	}

	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}

	s.setFlash(w, "Monitor deleted")
	s.redirect(w, r, "/monitors")
}

func (s *Server) handleWebMonitorPause(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	s.store.SetMonitorEnabled(r.Context(), id, false)
	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}
	s.setFlash(w, "Monitor paused")
	s.redirect(w, r, "/monitors/"+strconv.FormatInt(id, 10))
}

func (s *Server) handleWebMonitorResume(w http.ResponseWriter, r *http.Request) {
	id, _ := parseID(r)
	s.store.SetMonitorEnabled(r.Context(), id, true)
	if s.pipeline != nil {
		s.pipeline.ReloadMonitors()
	}
	s.setFlash(w, "Monitor resumed")
	s.redirect(w, r, "/monitors/"+strconv.FormatInt(id, 10))
}

func (s *Server) applyMonitorDefaults(m *storage.Monitor) {
	if m.Interval == 0 {
		m.Interval = int(s.cfg.Monitor.DefaultInterval.Seconds())
	}
	if m.Timeout == 0 {
		m.Timeout = int(s.cfg.Monitor.DefaultTimeout.Seconds())
	}
	if m.FailureThreshold == 0 {
		m.FailureThreshold = s.cfg.Monitor.FailureThreshold
	}
	if m.SuccessThreshold == 0 {
		m.SuccessThreshold = s.cfg.Monitor.SuccessThreshold
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	if m.Type == "heartbeat" && m.Target == "" {
		m.Target = "heartbeat"
	}
}

func (s *Server) parseMonitorForm(r *http.Request) *storage.Monitor {
	r.ParseForm()

	interval, _ := strconv.Atoi(r.FormValue("interval"))
	timeout, _ := strconv.Atoi(r.FormValue("timeout"))
	failThreshold, _ := strconv.Atoi(r.FormValue("failure_threshold"))
	successThreshold, _ := strconv.Atoi(r.FormValue("success_threshold"))

	mon := &storage.Monitor{
		Name:             r.FormValue("name"),
		Description:      r.FormValue("description"),
		Type:             r.FormValue("type"),
		Target:           r.FormValue("target"),
		Interval:         interval,
		Timeout:          timeout,
		Enabled:          true,
		TrackChanges:     r.FormValue("track_changes") == "on",
		Public:           r.FormValue("public") == "on",
		UpsideDown:       r.FormValue("upside_down") == "on",
		FailureThreshold: failThreshold,
		SuccessThreshold: successThreshold,
	}

	if v := r.FormValue("resend_interval"); v != "" {
		mon.ResendInterval, _ = strconv.Atoi(v)
	}

	if tags := strings.TrimSpace(r.FormValue("tags")); tags != "" {
		for _, t := range strings.Split(tags, ",") {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				mon.Tags = append(mon.Tags, trimmed)
			}
		}
	}

	if r.FormValue("settings_mode") == "json" {
		if raw := strings.TrimSpace(r.FormValue("settings_json")); raw != "" && json.Valid([]byte(raw)) {
			mon.Settings = json.RawMessage(raw)
		}
	} else {
		mon.Settings = assembleSettings(r, mon.Type)
	}

	if r.FormValue("assertions_mode") == "json" {
		if raw := strings.TrimSpace(r.FormValue("assertions_json")); raw != "" && json.Valid([]byte(raw)) {
			mon.Assertions = json.RawMessage(raw)
		}
	} else {
		mon.Assertions = assembleAssertions(r)
	}

	return mon
}
