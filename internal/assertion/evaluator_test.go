package assertion

import (
	"encoding/json"
	"testing"
)

func TestStatusCodeAssertion(t *testing.T) {
	assertions := []Assertion{
		{Type: "status_code", Operator: "eq", Value: "200"},
	}
	raw, _ := json.Marshal(assertions)

	result := Evaluate(raw, 200, "", nil, 100, nil, nil)
	if !result.Pass {
		t.Fatal("expected pass")
	}

	result = Evaluate(raw, 500, "", nil, 100, nil, nil)
	if result.Pass {
		t.Fatal("expected fail")
	}
}

func TestBodyContainsAssertion(t *testing.T) {
	assertions := []Assertion{
		{Type: "body_contains", Operator: "contains", Value: "hello"},
	}
	raw, _ := json.Marshal(assertions)

	result := Evaluate(raw, 200, "hello world", nil, 100, nil, nil)
	if !result.Pass {
		t.Fatal("expected pass")
	}

	result = Evaluate(raw, 200, "goodbye", nil, 100, nil, nil)
	if result.Pass {
		t.Fatal("expected fail")
	}
}

func TestBodyRegexAssertion(t *testing.T) {
	assertions := []Assertion{
		{Type: "body_regex", Operator: "matches", Value: `\d{3}`},
	}
	raw, _ := json.Marshal(assertions)

	result := Evaluate(raw, 200, "code 200 ok", nil, 100, nil, nil)
	if !result.Pass {
		t.Fatal("expected pass")
	}

	result = Evaluate(raw, 200, "no numbers", nil, 100, nil, nil)
	if result.Pass {
		t.Fatal("expected fail")
	}
}

func TestJSONPathAssertion(t *testing.T) {
	body := `{"status":"ok","data":{"count":42},"items":[{"id":1},{"id":2}]}`

	tests := []struct {
		target   string
		operator string
		value    string
		pass     bool
	}{
		{"status", "eq", "ok", true},
		{"data.count", "eq", "42", true},
		{"items[0].id", "eq", "1", true},
		{"items[1].id", "eq", "2", true},
		{"missing", "exists", "", false},
		{"status", "exists", "", true},
	}

	for _, tt := range tests {
		assertions := []Assertion{
			{Type: "json_path", Target: tt.target, Operator: tt.operator, Value: tt.value},
		}
		raw, _ := json.Marshal(assertions)
		result := Evaluate(raw, 200, body, nil, 100, nil, nil)
		if result.Pass != tt.pass {
			t.Fatalf("json_path %s %s %s: expected pass=%v, got %v (msg: %s)",
				tt.target, tt.operator, tt.value, tt.pass, result.Pass, result.Message)
		}
	}
}

func TestHeaderAssertion(t *testing.T) {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	assertions := []Assertion{
		{Type: "header", Target: "Content-Type", Operator: "contains", Value: "json"},
	}
	raw, _ := json.Marshal(assertions)

	result := Evaluate(raw, 200, "", headers, 100, nil, nil)
	if !result.Pass {
		t.Fatal("expected pass")
	}
}

func TestResponseTimeAssertion(t *testing.T) {
	assertions := []Assertion{
		{Type: "response_time", Operator: "lt", Value: "500"},
	}
	raw, _ := json.Marshal(assertions)

	result := Evaluate(raw, 200, "", nil, 200, nil, nil)
	if !result.Pass {
		t.Fatal("expected pass: 200 < 500")
	}

	result = Evaluate(raw, 200, "", nil, 600, nil, nil)
	if result.Pass {
		t.Fatal("expected fail: 600 < 500 should fail")
	}
}

func TestDegradedAssertion(t *testing.T) {
	assertions := []Assertion{
		{Type: "response_time", Operator: "lt", Value: "100", Degraded: true},
	}
	raw, _ := json.Marshal(assertions)

	result := Evaluate(raw, 200, "", nil, 200, nil, nil)
	if result.Pass {
		t.Fatal("expected fail")
	}
	if !result.Degraded {
		t.Fatal("expected degraded flag")
	}
}

func TestDNSRecordAssertion(t *testing.T) {
	records := []string{"1.2.3.4", "5.6.7.8"}

	assertions := []Assertion{
		{Type: "dns_record", Operator: "contains", Value: "1.2.3.4"},
	}
	raw, _ := json.Marshal(assertions)

	result := Evaluate(raw, 0, "", nil, 0, nil, records)
	if !result.Pass {
		t.Fatal("expected pass")
	}
}

func TestWalkJSONPath(t *testing.T) {
	jsonStr := `{"a":{"b":[1,2,3]}}`

	val, err := walkJSONPath(jsonStr, "a.b[1]")
	if err != nil {
		t.Fatal(err)
	}
	if val.(float64) != 2 {
		t.Fatalf("expected 2, got %v", val)
	}
}
