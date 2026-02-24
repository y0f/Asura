package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/y0f/asura/internal/assertion"
	"github.com/y0f/asura/internal/config"
	"github.com/y0f/asura/internal/storage"
)

func testWebHandler(t *testing.T) *Handler {
	t.Helper()
	cfg := config.Defaults()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return &Handler{
		cfg:    cfg,
		logger: logger,
	}
}

func buildFormRequest(values url.Values) *http.Request {
	r, _ := http.NewRequest("POST", "/", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ParseForm()
	return r
}

func TestAssembleHeaders(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		values []string
		want   int
	}{
		{"empty", nil, nil, 0},
		{"single", []string{"X-Custom"}, []string{"val"}, 1},
		{"multiple", []string{"A", "B"}, []string{"1", "2"}, 2},
		{"skip blank key", []string{"", "B"}, []string{"1", "2"}, 1},
		{"mismatched lengths", []string{"A", "B", "C"}, []string{"1"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			for _, k := range tt.keys {
				form.Add("hk[]", k)
			}
			for _, v := range tt.values {
				form.Add("hv[]", v)
			}
			r := buildFormRequest(form)
			got := assembleHeaders(r, "hk", "hv")
			if tt.want == 0 {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
			} else if len(got) != tt.want {
				t.Errorf("expected %d headers, got %d", tt.want, len(got))
			}
		})
	}
}

func TestAssembleSettingsHTTP(t *testing.T) {
	form := url.Values{
		"settings_method":          {"POST"},
		"settings_body":            {`{"key":"val"}`},
		"settings_body_encoding":   {"json"},
		"settings_auth_method":     {"bearer"},
		"settings_bearer_token":    {"my-token"},
		"settings_expected_status": {"201"},
		"settings_max_redirects":   {"5"},
		"settings_skip_tls_verify": {"on"},
		"settings_cache_buster":    {"on"},
	}
	form.Add("settings_header_key[]", "X-Test")
	form.Add("settings_header_value[]", "abc")

	r := buildFormRequest(form)
	raw := assembleSettings(r, "http")

	var s storage.HTTPSettings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}

	if s.Method != "POST" {
		t.Errorf("method = %q, want POST", s.Method)
	}
	if s.Body != `{"key":"val"}` {
		t.Errorf("body = %q", s.Body)
	}
	if s.BodyEncoding != "json" {
		t.Errorf("body_encoding = %q", s.BodyEncoding)
	}
	if s.AuthMethod != "bearer" {
		t.Errorf("auth_method = %q", s.AuthMethod)
	}
	if s.BearerToken != "my-token" {
		t.Errorf("bearer_token = %q", s.BearerToken)
	}
	if s.BasicAuthUser != "" {
		t.Errorf("basic_auth_user should be empty for bearer auth, got %q", s.BasicAuthUser)
	}
	if s.ExpectedStatus != 201 {
		t.Errorf("expected_status = %d", s.ExpectedStatus)
	}
	if s.MaxRedirects != 5 {
		t.Errorf("max_redirects = %d", s.MaxRedirects)
	}
	if !s.SkipTLSVerify {
		t.Error("skip_tls_verify should be true")
	}
	if !s.CacheBuster {
		t.Error("cache_buster should be true")
	}
	if s.Headers["X-Test"] != "abc" {
		t.Errorf("headers = %v", s.Headers)
	}
}

func TestAssembleSettingsHTTPBasicAuth(t *testing.T) {
	form := url.Values{
		"settings_method":          {"GET"},
		"settings_auth_method":     {"basic"},
		"settings_basic_auth_user": {"admin"},
		"settings_basic_auth_pass": {"secret"},
	}
	r := buildFormRequest(form)
	raw := assembleSettings(r, "http")

	var s storage.HTTPSettings
	json.Unmarshal(raw, &s)

	if s.BasicAuthUser != "admin" || s.BasicAuthPass != "secret" {
		t.Errorf("basic auth = %q / %q", s.BasicAuthUser, s.BasicAuthPass)
	}
	if s.BearerToken != "" {
		t.Errorf("bearer should be empty for basic auth, got %q", s.BearerToken)
	}
}

func TestAssembleSettingsHTTPMaxRedirectsZero(t *testing.T) {
	form := url.Values{
		"settings_method":        {"GET"},
		"settings_auth_method":   {"none"},
		"settings_max_redirects": {"0"},
	}
	r := buildFormRequest(form)
	raw := assembleSettings(r, "http")

	var s storage.HTTPSettings
	json.Unmarshal(raw, &s)

	if s.FollowRedirects == nil || *s.FollowRedirects != false {
		t.Error("follow_redirects should be false when max_redirects=0")
	}
}

func TestAssembleSettingsTCP(t *testing.T) {
	form := url.Values{
		"settings_send_data":   {"PING"},
		"settings_expect_data": {"PONG"},
	}
	r := buildFormRequest(form)
	raw := assembleSettings(r, "tcp")

	var s storage.TCPSettings
	json.Unmarshal(raw, &s)

	if s.SendData != "PING" || s.ExpectData != "PONG" {
		t.Errorf("tcp settings = %+v", s)
	}
}

func TestAssembleSettingsDNS(t *testing.T) {
	form := url.Values{
		"settings_record_type": {"MX"},
		"settings_dns_server":  {"8.8.8.8"},
	}
	r := buildFormRequest(form)
	raw := assembleSettings(r, "dns")

	var s storage.DNSSettings
	json.Unmarshal(raw, &s)

	if s.RecordType != "MX" || s.Server != "8.8.8.8" {
		t.Errorf("dns settings = %+v", s)
	}
}

func TestAssembleSettingsTLS(t *testing.T) {
	form := url.Values{"settings_warn_days_before": {"14"}}
	r := buildFormRequest(form)
	raw := assembleSettings(r, "tls")

	var s storage.TLSSettings
	json.Unmarshal(raw, &s)

	if s.WarnDaysBefore != 14 {
		t.Errorf("warn_days_before = %d", s.WarnDaysBefore)
	}
}

func TestAssembleSettingsWebSocket(t *testing.T) {
	form := url.Values{
		"settings_send_message": {"ping"},
		"settings_expect_reply": {"pong"},
	}
	form.Add("settings_ws_header_key[]", "Auth")
	form.Add("settings_ws_header_value[]", "tok")

	r := buildFormRequest(form)
	raw := assembleSettings(r, "websocket")

	var s storage.WebSocketSettings
	json.Unmarshal(raw, &s)

	if s.SendMessage != "ping" || s.ExpectReply != "pong" {
		t.Errorf("ws settings = %+v", s)
	}
	if s.Headers["Auth"] != "tok" {
		t.Errorf("ws headers = %v", s.Headers)
	}
}

func TestAssembleSettingsCommand(t *testing.T) {
	form := url.Values{
		"settings_command": {"/usr/bin/check"},
		"settings_args":    {"--host, db.local, --verbose"},
	}
	r := buildFormRequest(form)
	raw := assembleSettings(r, "command")

	var s storage.CommandSettings
	json.Unmarshal(raw, &s)

	if s.Command != "/usr/bin/check" {
		t.Errorf("command = %q", s.Command)
	}
	if len(s.Args) != 3 || s.Args[0] != "--host" || s.Args[1] != "db.local" || s.Args[2] != "--verbose" {
		t.Errorf("args = %v", s.Args)
	}
}

func TestAssembleSettingsUnknownType(t *testing.T) {
	r := buildFormRequest(url.Values{})
	raw := assembleSettings(r, "icmp")
	if raw != nil {
		t.Errorf("expected nil for icmp, got %s", raw)
	}
}

func TestAssembleAssertions(t *testing.T) {
	form := url.Values{
		"assertion_count":      {"2"},
		"assertion_type_0":     {"status_code"},
		"assertion_operator_0": {"eq"},
		"assertion_value_0":    {"200"},
		"assertion_type_1":     {"response_time"},
		"assertion_operator_1": {"lt"},
		"assertion_value_1":    {"5000"},
		"assertion_degraded_1": {"on"},
	}
	r := buildFormRequest(form)
	raw := assembleAssertions(r)

	var assertions []assertion.Assertion
	if err := json.Unmarshal(raw, &assertions); err != nil {
		t.Fatal(err)
	}

	if len(assertions) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(assertions))
	}
	if assertions[0].Type != "status_code" || assertions[0].Operator != "eq" || assertions[0].Value != "200" {
		t.Errorf("assertion[0] = %+v", assertions[0])
	}
	if assertions[1].Type != "response_time" || !assertions[1].Degraded {
		t.Errorf("assertion[1] = %+v", assertions[1])
	}
}

func TestAssembleAssertionsEmpty(t *testing.T) {
	r := buildFormRequest(url.Values{"assertion_count": {"0"}})
	raw := assembleAssertions(r)
	if raw != nil {
		t.Errorf("expected nil for 0 assertions, got %s", raw)
	}
}

func TestAssembleAssertionsSkipsEmptyType(t *testing.T) {
	form := url.Values{
		"assertion_count":      {"2"},
		"assertion_type_0":     {"status_code"},
		"assertion_operator_0": {"eq"},
		"assertion_value_0":    {"200"},
		"assertion_type_1":     {""},
		"assertion_operator_1": {"lt"},
		"assertion_value_1":    {"5000"},
	}
	r := buildFormRequest(form)
	raw := assembleAssertions(r)

	var assertions []assertion.Assertion
	json.Unmarshal(raw, &assertions)

	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion (skipping empty type), got %d", len(assertions))
	}
}

func TestAssembleAssertionsCap(t *testing.T) {
	form := url.Values{"assertion_count": {"100"}}
	for i := 0; i < 100; i++ {
		idx := strconv.Itoa(i)
		form.Set("assertion_type_"+idx, "status_code")
		form.Set("assertion_operator_"+idx, "eq")
		form.Set("assertion_value_"+idx, "200")
	}
	r := buildFormRequest(form)
	raw := assembleAssertions(r)

	var assertions []assertion.Assertion
	json.Unmarshal(raw, &assertions)

	if len(assertions) != 50 {
		t.Fatalf("expected cap at 50, got %d", len(assertions))
	}
}

func TestMonitorToFormDataNil(t *testing.T) {
	fd := monitorToFormData(nil)

	if fd.Monitor == nil {
		t.Fatal("Monitor should not be nil")
	}
	if !fd.FollowRedirects {
		t.Error("FollowRedirects should default to true")
	}
	if fd.MaxRedirects != 10 {
		t.Errorf("MaxRedirects = %d, want 10", fd.MaxRedirects)
	}
	if fd.HTTP.AuthMethod != "none" {
		t.Errorf("AuthMethod = %q, want none", fd.HTTP.AuthMethod)
	}
	if string(fd.HeadersJSON) != "[]" {
		t.Errorf("HeadersJSON = %s, want []", fd.HeadersJSON)
	}
}

func TestMonitorToFormDataHTTP(t *testing.T) {
	settings, _ := json.Marshal(storage.HTTPSettings{
		Method:        "POST",
		Headers:       map[string]string{"X-A": "1"},
		BasicAuthUser: "admin",
		BasicAuthPass: "pass",
	})
	assertions, _ := json.Marshal([]assertion.Assertion{
		{Type: "status_code", Operator: "eq", Value: "200"},
	})
	mon := &storage.Monitor{
		ID:         1,
		Type:       "http",
		Settings:   settings,
		Assertions: assertions,
	}
	fd := monitorToFormData(mon)

	if fd.HTTP.Method != "POST" {
		t.Errorf("Method = %q", fd.HTTP.Method)
	}
	if fd.HTTP.AuthMethod != "basic" {
		t.Errorf("AuthMethod = %q, want basic (inferred)", fd.HTTP.AuthMethod)
	}
	if string(fd.HeadersJSON) == "[]" {
		t.Error("HeadersJSON should not be empty")
	}
	if string(fd.AssertionsJSON) == "[]" {
		t.Error("AssertionsJSON should not be empty")
	}
}

func TestMonitorToFormDataBearerInference(t *testing.T) {
	settings, _ := json.Marshal(storage.HTTPSettings{BearerToken: "tok123"})
	mon := &storage.Monitor{Type: "http", Settings: settings}
	fd := monitorToFormData(mon)

	if fd.HTTP.AuthMethod != "bearer" {
		t.Errorf("AuthMethod = %q, want bearer (inferred)", fd.HTTP.AuthMethod)
	}
}

func TestParseMonitorFormFormMode(t *testing.T) {
	form := url.Values{
		"name":                 {"My Monitor"},
		"description":          {"A test monitor"},
		"type":                 {"http"},
		"target":               {"https://example.com"},
		"interval":             {"30"},
		"timeout":              {"5"},
		"failure_threshold":    {"3"},
		"success_threshold":    {"1"},
		"tags":                 {"prod, api"},
		"track_changes":        {"on"},
		"upside_down":          {"on"},
		"resend_interval":      {"60"},
		"settings_mode":        {"form"},
		"settings_method":      {"GET"},
		"settings_auth_method": {"none"},
		"assertions_mode":      {"form"},
		"assertion_count":      {"1"},
		"assertion_type_0":     {"status_code"},
		"assertion_operator_0": {"eq"},
		"assertion_value_0":    {"200"},
	}

	h := testWebHandler(t)
	r := buildFormRequest(form)
	mon, _ := h.parseMonitorForm(r)

	if mon.Name != "My Monitor" {
		t.Errorf("Name = %q", mon.Name)
	}
	if mon.Description != "A test monitor" {
		t.Errorf("Description = %q", mon.Description)
	}
	if !mon.UpsideDown {
		t.Error("UpsideDown should be true")
	}
	if mon.ResendInterval != 60 {
		t.Errorf("ResendInterval = %d", mon.ResendInterval)
	}
	if len(mon.Tags) != 2 || mon.Tags[0] != "prod" || mon.Tags[1] != "api" {
		t.Errorf("Tags = %v", mon.Tags)
	}
	if mon.Settings == nil {
		t.Error("Settings should not be nil")
	}
	if mon.Assertions == nil {
		t.Error("Assertions should not be nil")
	}
}

func TestParseMonitorFormJSONMode(t *testing.T) {
	form := url.Values{
		"name":              {"JSON Monitor"},
		"type":              {"http"},
		"target":            {"https://example.com"},
		"interval":          {"60"},
		"timeout":           {"10"},
		"failure_threshold": {"3"},
		"success_threshold": {"1"},
		"settings_mode":     {"json"},
		"settings_json":     {`{"method":"PUT","body":"test"}`},
		"assertions_mode":   {"json"},
		"assertions_json":   {`[{"type":"status_code","operator":"eq","value":"200"}]`},
	}

	h := testWebHandler(t)
	r := buildFormRequest(form)
	mon, _ := h.parseMonitorForm(r)

	var s storage.HTTPSettings
	json.Unmarshal(mon.Settings, &s)
	if s.Method != "PUT" {
		t.Errorf("Settings method = %q, want PUT", s.Method)
	}

	var a []assertion.Assertion
	json.Unmarshal(mon.Assertions, &a)
	if len(a) != 1 {
		t.Errorf("expected 1 assertion, got %d", len(a))
	}
}

func TestParseMonitorFormJSONModeInvalidJSON(t *testing.T) {
	form := url.Values{
		"name":              {"Bad JSON"},
		"type":              {"http"},
		"target":            {"https://example.com"},
		"interval":          {"60"},
		"timeout":           {"10"},
		"failure_threshold": {"3"},
		"success_threshold": {"1"},
		"settings_mode":     {"json"},
		"settings_json":     {`{invalid`},
		"assertions_mode":   {"json"},
		"assertions_json":   {`[broken`},
	}

	h := testWebHandler(t)
	r := buildFormRequest(form)
	mon, _ := h.parseMonitorForm(r)

	if mon.Settings != nil {
		t.Errorf("invalid JSON settings should be nil, got %s", mon.Settings)
	}
	if mon.Assertions != nil {
		t.Errorf("invalid JSON assertions should be nil, got %s", mon.Assertions)
	}
}
