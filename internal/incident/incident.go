package incident

const (
	StatusOpen         = "open"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"
)

const (
	EventCreated        = "created"
	EventAcknowledged   = "acknowledged"
	EventResolved       = "resolved"
	EventCheckFailed    = "check_failed"
	EventCheckRecovered = "check_recovered"
)
