package assertion

type Assertion struct {
	Type     string `json:"type"`
	Operator string `json:"operator"`
	Target   string `json:"target"`
	Value    string `json:"value"`
	Degraded bool   `json:"degraded"`
}

type ConditionGroup struct {
	Operator   string      `json:"operator"`
	Conditions []Assertion `json:"conditions"`
}

type ConditionSet struct {
	Operator string           `json:"operator"`
	Groups   []ConditionGroup `json:"groups"`
}

type AssertionResult struct {
	Pass     bool
	Degraded bool
	Message  string
	Details  []AssertionDetail
}

type AssertionDetail struct {
	Assertion Assertion
	Pass      bool
	Actual    string
	Message   string
}
