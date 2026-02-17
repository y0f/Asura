package incident

// Incident status constants.
const (
	StatusOpen         = "open"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"
)

// Event type constants.
const (
	EventCreated        = "created"
	EventAcknowledged   = "acknowledged"
	EventResolved       = "resolved"
	EventCheckFailed    = "check_failed"
	EventCheckRecovered = "check_recovered"
)
