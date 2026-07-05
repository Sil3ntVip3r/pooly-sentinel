package storage

import "time"

type MetadataRecord struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

type CollectorStateRecord struct {
	Collector        string
	Target           string
	Status           string
	StateJSON        string
	LastAttemptAt    *time.Time
	LastSuccessAt    *time.Time
	LastErrorClass   string
	LastErrorSummary string
	UpdatedAt        time.Time
}

type IncidentRecord struct {
	ID              string
	Fingerprint     string
	NodeID          string
	Type            string
	Target          string
	Condition       string
	Severity        string
	Status          string
	Summary         string
	FirstSeen       time.Time
	LastSeen        time.Time
	LastAlerted     *time.Time
	OccurrenceCount int64
	EvidencePath    string
	ResolvedAt      *time.Time
	LastTransition  *time.Time
	UpdatedAt       time.Time
}

type NotificationDeliveryRecord struct {
	ID           string
	IncidentID   string
	Receiver     string
	CostClass    string
	Status       string
	Attempt      int
	AttemptedAt  time.Time
	DeliveredAt  *time.Time
	ErrorClass   string
	ErrorSummary string
}

type RuleEvaluationStateRecord struct {
	RuleID            string
	Target            string
	State             string
	Severity          string
	ConditionMetSince *time.Time
	RecoverySince     *time.Time
	LastEvaluatedAt   time.Time
	LastObservedAt    *time.Time
	LastResultSummary string
	PendingSeverity   string
	UpdatedAt         time.Time
}
