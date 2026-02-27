package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/y0f/asura/internal/assertion"
	"github.com/y0f/asura/internal/httputil"
	"github.com/y0f/asura/internal/storage"
	"github.com/y0f/asura/internal/validate"
)

type headerPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type monitorFormData struct {
	Monitor              *storage.Monitor
	HTTP                 storage.HTTPSettings
	TCP                  storage.TCPSettings
	DNS                  storage.DNSSettings
	TLS                  storage.TLSSettings
	WS                   storage.WebSocketSettings
	Cmd                  storage.CommandSettings
	Docker               storage.DockerSettings
	Domain               storage.DomainSettings
	GRPC                 storage.GRPCSettings
	MQTT                 storage.MQTTSettings
	FollowRedirects      bool
	MaxRedirects         int
	HeadersJSON          template.JS
	WsHeadersJSON        template.JS
	AssertionsJSON       template.JS
	SettingsJSON         string
	AssertionsRaw        string
	Groups               []*storage.MonitorGroup
	NotificationChannels []*storage.NotificationChannel
	SelectedChannelIDs   []int64
	Proxies              []*storage.Proxy
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

	fd.SettingsJSON = "{}"
	if len(mon.Settings) > 0 {
		fd.SettingsJSON = string(mon.Settings)
	}
	fd.AssertionsRaw = "[]"
	if len(mon.Assertions) > 0 {
		fd.AssertionsRaw = string(mon.Assertions)
	}

	unmarshalMonitorSettings(fd, mon)
	applyHTTPDefaults(fd)

	fd.HeadersJSON = headersToJSON(fd.HTTP.Headers)
	fd.WsHeadersJSON = headersToJSON(fd.WS.Headers)
	fd.AssertionsJSON = assertionsToJSON(mon.Assertions)
	return fd
}

var _settingsTargets = map[string]func(*monitorFormData) any{
	"http":      func(fd *monitorFormData) any { return &fd.HTTP },
	"tcp":       func(fd *monitorFormData) any { return &fd.TCP },
	"dns":       func(fd *monitorFormData) any { return &fd.DNS },
	"tls":       func(fd *monitorFormData) any { return &fd.TLS },
	"websocket": func(fd *monitorFormData) any { return &fd.WS },
	"command":   func(fd *monitorFormData) any { return &fd.Cmd },
	"docker":    func(fd *monitorFormData) any { return &fd.Docker },
	"domain":    func(fd *monitorFormData) any { return &fd.Domain },
	"grpc":      func(fd *monitorFormData) any { return &fd.GRPC },
	"mqtt":      func(fd *monitorFormData) any { return &fd.MQTT },
}

func unmarshalMonitorSettings(fd *monitorFormData, mon *storage.Monitor) {
	if fn, ok := _settingsTargets[mon.Type]; ok {
		json.Unmarshal(mon.Settings, fn(fd))
	}
}

func applyHTTPDefaults(fd *monitorFormData) {
	fd.FollowRedirects = fd.HTTP.FollowRedirects == nil || *fd.HTTP.FollowRedirects
	fd.MaxRedirects = fd.HTTP.MaxRedirects
	if fd.MaxRedirects == 0 && fd.FollowRedirects {
		fd.MaxRedirects = 10
	}
	fd.HTTP.AuthMethod = inferHTTPAuthMethod(fd.HTTP)
	if fd.HTTP.BodyEncoding == "" {
		fd.HTTP.BodyEncoding = "json"
	}
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
	return safeJS(b)
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
	return safeJS(b)
}

var _settingsAssemblers = map[string]func(*http.Request) json.RawMessage{
	"http": func(r *http.Request) json.RawMessage {
		b, _ := json.Marshal(assembleHTTPSettings(r))
		return b
	},
	"tcp": func(r *http.Request) json.RawMessage {
		b, _ := json.Marshal(storage.TCPSettings{
			SendData:   r.FormValue("settings_send_data"),
			ExpectData: r.FormValue("settings_expect_data"),
		})
		return b
	},
	"dns": func(r *http.Request) json.RawMessage {
		b, _ := json.Marshal(storage.DNSSettings{
			RecordType: r.FormValue("settings_record_type"),
			Server:     r.FormValue("settings_dns_server"),
		})
		return b
	},
	"tls": func(r *http.Request) json.RawMessage {
		s := storage.TLSSettings{}
		if v := r.FormValue("settings_warn_days_before"); v != "" {
			s.WarnDaysBefore, _ = strconv.Atoi(v)
		}
		b, _ := json.Marshal(s)
		return b
	},
	"websocket": func(r *http.Request) json.RawMessage {
		b, _ := json.Marshal(storage.WebSocketSettings{
			SendMessage: r.FormValue("settings_send_message"),
			ExpectReply: r.FormValue("settings_expect_reply"),
			Headers:     assembleHeaders(r, "settings_ws_header_key", "settings_ws_header_value"),
		})
		return b
	},
	"command": assembleCommandSettings,
	"docker": func(r *http.Request) json.RawMessage {
		b, _ := json.Marshal(storage.DockerSettings{
			ContainerName: r.FormValue("settings_container_name"),
			SocketPath:    r.FormValue("settings_socket_path"),
			CheckHealth:   r.FormValue("settings_check_health") == "on",
		})
		return b
	},
	"domain": func(r *http.Request) json.RawMessage {
		s := storage.DomainSettings{}
		if v := r.FormValue("settings_domain_warn_days"); v != "" {
			s.WarnDaysBefore, _ = strconv.Atoi(v)
		}
		b, _ := json.Marshal(s)
		return b
	},
	"grpc": func(r *http.Request) json.RawMessage {
		b, _ := json.Marshal(storage.GRPCSettings{
			ServiceName:   r.FormValue("settings_grpc_service"),
			UseTLS:        r.FormValue("settings_grpc_tls") == "on",
			SkipTLSVerify: r.FormValue("settings_grpc_skip_verify") == "on",
		})
		return b
	},
	"mqtt": func(r *http.Request) json.RawMessage {
		b, _ := json.Marshal(storage.MQTTSettings{
			ClientID:      r.FormValue("settings_mqtt_client_id"),
			Username:      r.FormValue("settings_mqtt_username"),
			Password:      r.FormValue("settings_mqtt_password"),
			Topic:         r.FormValue("settings_mqtt_topic"),
			ExpectMessage: r.FormValue("settings_mqtt_expect"),
			UseTLS:        r.FormValue("settings_mqtt_tls") == "on",
		})
		return b
	},
}

func assembleSettings(r *http.Request, monType string) json.RawMessage {
	if fn, ok := _settingsAssemblers[monType]; ok {
		return fn(r)
	}
	return nil
}

func assembleCommandSettings(r *http.Request) json.RawMessage {
	s := storage.CommandSettings{Command: r.FormValue("settings_command")}
	if argsStr := strings.TrimSpace(r.FormValue("settings_args")); argsStr != "" {
		for _, a := range strings.Split(argsStr, ",") {
			if trimmed := strings.TrimSpace(a); trimmed != "" {
				s.Args = append(s.Args, trimmed)
			}
		}
	}
	b, _ := json.Marshal(s)
	return b
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

func (h *Handler) Monitors(w http.ResponseWriter, r *http.Request) {
	p := httputil.ParsePagination(r)
	if p.PerPage == 20 {
		p.PerPage = 15
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	typeFilter := r.URL.Query().Get("type")
	if !validate.ValidMonitorTypes[typeFilter] {
		typeFilter = ""
	}

	f := storage.MonitorListFilter{Type: typeFilter, Search: q}
	result, err := h.store.ListMonitors(r.Context(), f, p)
	if err != nil {
		h.logger.Error("web: list monitors", "error", err)
	}

	groups, _ := h.store.ListMonitorGroups(r.Context())

	pd := h.newPageData(r, "Monitors", "monitors")
	pd.Data = map[string]any{
		"Result": result,
		"Search": q,
		"Type":   typeFilter,
		"Groups": groups,
	}
	h.render(w, "monitors/list.html", pd)
}

func (h *Handler) MonitorDetail(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		h.redirect(w, r, "/monitors")
		return
	}

	ctx := r.Context()
	mon, err := h.store.GetMonitor(ctx, id)
	if err != nil {
		h.redirect(w, r, "/monitors")
		return
	}

	checksPage, _ := strconv.Atoi(r.URL.Query().Get("checks_page"))
	if checksPage < 1 {
		checksPage = 1
	}
	changesPage, _ := strconv.Atoi(r.URL.Query().Get("changes_page"))
	if changesPage < 1 {
		changesPage = 1
	}

	now := time.Now().UTC()
	checks, _ := h.store.ListCheckResults(ctx, id, storage.Pagination{Page: checksPage, PerPage: 10})
	if checks == nil {
		checks = &storage.PaginatedResult{}
	}
	changes, _ := h.store.ListContentChanges(ctx, id, storage.Pagination{Page: changesPage, PerPage: 10})
	if changes == nil {
		changes = &storage.PaginatedResult{}
	}

	uptime24h, _ := h.store.GetUptimePercent(ctx, id, now.Add(-24*time.Hour), now)
	uptime7d, _ := h.store.GetUptimePercent(ctx, id, now.Add(-7*24*time.Hour), now)
	uptime30d, _ := h.store.GetUptimePercent(ctx, id, now.Add(-30*24*time.Hour), now)
	p50, p95, p99, _ := h.store.GetResponseTimePercentiles(ctx, id, now.Add(-24*time.Hour), now)
	totalChecks, upChecks, downChecks, _, _ := h.store.GetCheckCounts(ctx, id, now.Add(-24*time.Hour), now)
	latestCheck, _ := h.store.GetLatestCheckResult(ctx, id)
	openIncident, _ := h.store.GetOpenIncident(ctx, id)

	pd := h.newPageData(r, mon.Name, "monitors")
	pd.Data = map[string]any{
		"Monitor":      mon,
		"Checks":       checks,
		"Changes":      changes,
		"ChecksPage":   checksPage,
		"ChangesPage":  changesPage,
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
	h.render(w, "monitors/detail.html", pd)
}

func (h *Handler) MonitorForm(w http.ResponseWriter, r *http.Request) {
	pd := h.newPageData(r, "New Monitor", "monitors")

	groups, _ := h.store.ListMonitorGroups(r.Context())
	channels, _ := h.store.ListNotificationChannels(r.Context())
	proxies, _ := h.store.ListProxies(r.Context())

	idStr := r.PathValue("id")
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			h.redirect(w, r, "/monitors")
			return
		}
		mon, err := h.store.GetMonitor(r.Context(), id)
		if err != nil {
			h.redirect(w, r, "/monitors")
			return
		}
		pd.Title = "Edit " + mon.Name
		fd := monitorToFormData(mon)
		fd.Groups = groups
		fd.NotificationChannels = channels
		fd.Proxies = proxies
		fd.SelectedChannelIDs, _ = h.store.GetMonitorNotificationChannelIDs(r.Context(), id)
		pd.Data = fd
	} else {
		fd := monitorToFormData(nil)
		fd.Groups = groups
		fd.NotificationChannels = channels
		fd.Proxies = proxies
		pd.Data = fd
	}

	h.render(w, "monitors/form.html", pd)
}

func (h *Handler) MonitorCreate(w http.ResponseWriter, r *http.Request) {
	mon, channelIDs := h.parseMonitorForm(r)

	h.applyMonitorDefaults(mon)

	if err := validate.ValidateMonitor(mon); err != nil {
		groups, _ := h.store.ListMonitorGroups(r.Context())
		channels, _ := h.store.ListNotificationChannels(r.Context())
		proxies, _ := h.store.ListProxies(r.Context())
		pd := h.newPageData(r, "New Monitor", "monitors")
		pd.Error = err.Error()
		fd := monitorToFormData(mon)
		fd.Groups = groups
		fd.NotificationChannels = channels
		fd.Proxies = proxies
		fd.SelectedChannelIDs = channelIDs
		pd.Data = fd
		h.render(w, "monitors/form.html", pd)
		return
	}

	if err := h.store.CreateMonitor(r.Context(), mon); err != nil {
		groups, _ := h.store.ListMonitorGroups(r.Context())
		channels, _ := h.store.ListNotificationChannels(r.Context())
		proxies, _ := h.store.ListProxies(r.Context())
		h.logger.Error("web: create monitor", "error", err)
		pd := h.newPageData(r, "New Monitor", "monitors")
		pd.Error = "Failed to create monitor"
		fd := monitorToFormData(mon)
		fd.Groups = groups
		fd.NotificationChannels = channels
		fd.Proxies = proxies
		fd.SelectedChannelIDs = channelIDs
		pd.Data = fd
		h.render(w, "monitors/form.html", pd)
		return
	}

	if len(channelIDs) > 0 {
		if err := h.store.SetMonitorNotificationChannels(r.Context(), mon.ID, channelIDs); err != nil {
			h.logger.Error("web: set monitor notification channels", "error", err)
		}
	}

	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}

	h.setFlash(w, "Monitor created")
	h.redirect(w, r, "/monitors")
}

func (h *Handler) MonitorUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		h.redirect(w, r, "/monitors")
		return
	}

	mon, channelIDs := h.parseMonitorForm(r)
	mon.ID = id

	if err := validate.ValidateMonitor(mon); err != nil {
		groups, _ := h.store.ListMonitorGroups(r.Context())
		channels, _ := h.store.ListNotificationChannels(r.Context())
		proxies, _ := h.store.ListProxies(r.Context())
		pd := h.newPageData(r, "Edit Monitor", "monitors")
		pd.Error = err.Error()
		fd := monitorToFormData(mon)
		fd.Groups = groups
		fd.NotificationChannels = channels
		fd.Proxies = proxies
		fd.SelectedChannelIDs = channelIDs
		pd.Data = fd
		h.render(w, "monitors/form.html", pd)
		return
	}

	if err := h.store.UpdateMonitor(r.Context(), mon); err != nil {
		groups, _ := h.store.ListMonitorGroups(r.Context())
		channels, _ := h.store.ListNotificationChannels(r.Context())
		proxies, _ := h.store.ListProxies(r.Context())
		h.logger.Error("web: update monitor", "error", err)
		pd := h.newPageData(r, "Edit Monitor", "monitors")
		pd.Error = "Failed to update monitor"
		fd := monitorToFormData(mon)
		fd.Groups = groups
		fd.NotificationChannels = channels
		fd.Proxies = proxies
		fd.SelectedChannelIDs = channelIDs
		pd.Data = fd
		h.render(w, "monitors/form.html", pd)
		return
	}

	if err := h.store.SetMonitorNotificationChannels(r.Context(), id, channelIDs); err != nil {
		h.logger.Error("web: set monitor notification channels", "error", err)
	}

	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}

	h.setFlash(w, "Monitor updated")
	h.redirect(w, r, "/monitors/"+strconv.FormatInt(id, 10))
}

func (h *Handler) MonitorDelete(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseID(r)
	if err != nil {
		h.redirect(w, r, "/monitors")
		return
	}

	if err := h.store.DeleteMonitor(r.Context(), id); err != nil {
		h.logger.Error("web: delete monitor", "error", err)
	}

	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}

	h.setFlash(w, "Monitor deleted")
	h.redirect(w, r, "/monitors")
}

func (h *Handler) MonitorPause(w http.ResponseWriter, r *http.Request) {
	id, _ := httputil.ParseID(r)
	h.store.SetMonitorEnabled(r.Context(), id, false)
	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}
	h.setFlash(w, "Monitor paused")
	h.redirect(w, r, "/monitors/"+strconv.FormatInt(id, 10))
}

func (h *Handler) MonitorResume(w http.ResponseWriter, r *http.Request) {
	id, _ := httputil.ParseID(r)
	h.store.SetMonitorEnabled(r.Context(), id, true)
	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}
	h.setFlash(w, "Monitor resumed")
	h.redirect(w, r, "/monitors/"+strconv.FormatInt(id, 10))
}

func (h *Handler) MonitorBulk(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	action := r.FormValue("action")
	ids := parseIDList(r.Form["ids[]"])
	if len(ids) == 0 {
		h.setFlash(w, "No monitors selected")
		h.redirect(w, r, "/monitors")
		return
	}

	ctx := r.Context()
	var msg string

	switch action {
	case "pause":
		h.store.BulkSetMonitorsEnabled(ctx, ids, false)
		msg = strconv.Itoa(len(ids)) + " monitors paused"
	case "resume":
		h.store.BulkSetMonitorsEnabled(ctx, ids, true)
		msg = strconv.Itoa(len(ids)) + " monitors resumed"
	case "delete":
		h.store.BulkDeleteMonitors(ctx, ids)
		msg = strconv.Itoa(len(ids)) + " monitors deleted"
	case "set_group":
		var gid *int64
		if v := r.FormValue("group_id"); v != "" {
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil && parsed > 0 {
				gid = &parsed
			}
		}
		h.store.BulkSetMonitorGroup(ctx, ids, gid)
		msg = strconv.Itoa(len(ids)) + " monitors updated"
	default:
		h.setFlash(w, "Invalid action")
		h.redirect(w, r, "/monitors")
		return
	}

	if h.pipeline != nil {
		h.pipeline.ReloadMonitors()
	}
	h.setFlash(w, msg)
	h.redirect(w, r, "/monitors")
}

func (h *Handler) applyMonitorDefaults(m *storage.Monitor) {
	if m.Interval == 0 {
		m.Interval = int(h.cfg.Monitor.DefaultInterval.Seconds())
	}
	if m.Timeout == 0 {
		m.Timeout = int(h.cfg.Monitor.DefaultTimeout.Seconds())
	}
	if m.FailureThreshold == 0 {
		m.FailureThreshold = h.cfg.Monitor.FailureThreshold
	}
	if m.SuccessThreshold == 0 {
		m.SuccessThreshold = h.cfg.Monitor.SuccessThreshold
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	if m.Type == "heartbeat" && m.Target == "" {
		m.Target = "heartbeat"
	}
}

func (h *Handler) parseMonitorForm(r *http.Request) (*storage.Monitor, []int64) {
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
		UpsideDown:       r.FormValue("upside_down") == "on",
		FailureThreshold: failThreshold,
		SuccessThreshold: successThreshold,
	}

	if v := r.FormValue("resend_interval"); v != "" {
		mon.ResendInterval, _ = strconv.Atoi(v)
	}

	if v := r.FormValue("group_id"); v != "" {
		gid, err := strconv.ParseInt(v, 10, 64)
		if err == nil && gid > 0 {
			mon.GroupID = &gid
		}
	}

	if v := r.FormValue("proxy_id"); v != "" {
		pid, err := strconv.ParseInt(v, 10, 64)
		if err == nil && pid > 0 {
			mon.ProxyID = &pid
		}
	}

	mon.Tags = parseCSV(r.FormValue("tags"))
	mon.Settings = parseJSONOrForm(r, "settings", func(r *http.Request) json.RawMessage {
		return assembleSettings(r, mon.Type)
	})
	mon.Assertions = parseJSONOrForm(r, "assertions", func(r *http.Request) json.RawMessage {
		return assembleAssertions(r)
	})

	return mon, parseIDList(r.Form["notification_channel_ids[]"])
}

func parseCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var result []string
	for _, t := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseJSONOrForm(r *http.Request, prefix string, formFn func(*http.Request) json.RawMessage) json.RawMessage {
	if r.FormValue(prefix+"_mode") == "json" {
		if raw := strings.TrimSpace(r.FormValue(prefix + "_json")); raw != "" && json.Valid([]byte(raw)) {
			return json.RawMessage(raw)
		}
		return nil
	}
	return formFn(r)
}

func parseIDList(values []string) []int64 {
	var ids []int64
	for _, v := range values {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
