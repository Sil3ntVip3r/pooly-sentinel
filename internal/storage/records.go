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
