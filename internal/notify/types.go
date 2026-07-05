package notify

import (
	"context"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type Event string

const (
	EventOpened    Event = "opened"
	EventEscalated Event = "escalated"
	EventResolved  Event = "resolved"
	EventReopened  Event = "reopened"
)

type Status string

const (
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
	StatusDryRun    Status = "dry_run"
	StatusSkipped   Status = "skipped"
)

type ReceiverSpec struct {
	ID                 string
	DisplayName        string
	Enabled            bool
	Type               string
	URL                string
	URLConfigured      bool
	Timeout            time.Duration
	Events             []Event
	Severities         []incidents.Severity
	AllowInsecureLocal bool
	CostClass          string
}

type Payload struct {
	Event           Event      `json:"event"`
	IncidentID      string     `json:"incident_id"`
	Fingerprint     string     `json:"fingerprint,omitempty"`
	NodeID          string     `json:"node_id"`
	Type            string     `json:"type"`
	Target          string     `json:"target"`
	Condition       string     `json:"condition"`
	Severity        string     `json:"severity"`
	Status          string     `json:"status"`
	Summary         string     `json:"summary"`
	FirstSeen       time.Time  `json:"first_seen"`
	LastSeen        time.Time  `json:"last_seen"`
	OccurrenceCount int64      `json:"occurrence_count"`
	LastTransition  *time.Time `json:"last_transition,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	EvidencePath    string     `json:"evidence_path,omitempty"`
}

type DeliveryResult struct {
	IncidentID string `json:"incident_id"`
	ReceiverID string `json:"receiver_id"`
	Event      Event  `json:"event"`
	Status     Status `json:"status"`
	Attempt    int    `json:"attempt"`
	ErrorClass string `json:"error_class,omitempty"`
	Summary    string `json:"summary"`
}

type Report struct {
	Results   []DeliveryResult `json:"results"`
	Delivered int              `json:"delivered"`
	Failed    int              `json:"failed"`
	Skipped   int              `json:"skipped"`
	DryRun    int              `json:"dry_run"`
}

type Receiver interface {
	ID() string
	Enabled() bool
	Type() string
	CostClass() string
	Matches(Event, incidents.Severity) bool
	Summary() string
	Deliver(ctx context.Context, payload Payload) DeliveryOutcome
}

type DeliveryOutcome struct {
	Success    bool
	ErrorClass string
	Summary    string
}

type Store interface {
	GetIncident(ctx context.Context, id string) (storage.IncidentRecord, error)
	ListNotificationDeliveries(ctx context.Context, incidentID string) ([]storage.NotificationDeliveryRecord, error)
	NotificationDeliveryTransaction(ctx context.Context, fn func(storage.NotificationDeliveryTransaction) error) error
}

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now().UTC()
}
