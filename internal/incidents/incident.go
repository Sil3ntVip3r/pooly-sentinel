package incidents

import "time"

type Severity string

const (
	SeverityNone     Severity = "none"
	SeverityWarning  Severity = "warning"
	SeverityFailure  Severity = "failure"
	SeverityCritical Severity = "critical"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusResolved Status = "resolved"
)

type Action string

const (
	ActionNone      Action = "none"
	ActionOpened    Action = "opened"
	ActionUpdated   Action = "updated"
	ActionEscalated Action = "escalated"
	ActionResolved  Action = "resolved"
	ActionReopened  Action = "reopened"
)

type Candidate struct {
	NodeID       string
	Type         string
	Target       string
	Condition    string
	Severity     Severity
	Active       bool
	Summary      string
	EvidencePath string
	ObservedAt   time.Time
}

type Transition struct {
	Action      Action
	IncidentID  string
	Fingerprint string
	Severity    Severity
	Status      Status
	Summary     string
}
