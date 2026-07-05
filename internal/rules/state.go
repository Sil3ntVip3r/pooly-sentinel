package rules

import (
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
)

type Severity = incidents.Severity

const (
	SeverityNone     = incidents.SeverityNone
	SeverityWarning  = incidents.SeverityWarning
	SeverityFailure  = incidents.SeverityFailure
	SeverityCritical = incidents.SeverityCritical
)

type State string

const (
	StateOK          State = "OK"
	StatePendingWarn State = "PENDING_WARN"
	StateWarn        State = "WARN"
	StatePendingFail State = "PENDING_FAIL"
	StateFail        State = "FAIL"
	StateCritical    State = "CRITICAL"
	StateRecovering  State = "RECOVERING"
	StateRecovered   State = "RECOVERED"
	StateStale       State = "STALE"
	StateUnknown     State = "UNKNOWN"
)

type Policy string

const (
	PolicyIgnore Policy = "ignore"
	PolicyStale  Policy = "stale"
	PolicyWarn   Policy = "warn"
	PolicyFail   Policy = "fail"
)

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now().UTC()
}

type StaticClock struct {
	At time.Time
}

func (c StaticClock) Now() time.Time {
	return c.At.UTC()
}

type EvaluationMemory struct {
	State             State
	Severity          Severity
	ConditionMetSince *time.Time
	RecoverySince     *time.Time
	LastEvaluatedAt   time.Time
	LastObservedAt    *time.Time
	LastResultSummary string
	PendingSeverity   Severity
}
