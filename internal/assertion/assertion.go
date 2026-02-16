package assertion

// Assertion defines a single check condition.
type Assertion struct {
	Type     string `json:"type"`     // status_code, body_contains, body_regex, json_path, header, response_time, cert_expiry, dns_record
	Operator string `json:"operator"` // eq, neq, gt, lt, gte, lte, contains, not_contains, matches, exists
	Target   string `json:"target"`   // what to check (e.g., header name, json path)
	Value    string `json:"value"`    // expected value
	Degraded bool   `json:"degraded"` // if true, failure marks as degraded instead of down
}

// AssertionResult holds the outcome of evaluating assertions.
type AssertionResult struct {
	Pass     bool
	Degraded bool
	Message  string
	Details  []AssertionDetail
}

// AssertionDetail holds the result of a single assertion.
type AssertionDetail struct {
	Assertion Assertion
	Pass      bool
	Actual    string
	Message   string
}
