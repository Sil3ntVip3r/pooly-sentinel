package reports

import (
	"context"
	"sort"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/agent"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type Store interface {
	Ping(ctx context.Context) error
	SchemaVersion(ctx context.Context) (int, error)
	IncidentStatusCounts(ctx context.Context) (map[string]int64, error)
	NotificationDeliveryStatusCounts(ctx context.Context) (map[string]int64, error)
	ListRecentIncidents(ctx context.Context, limit int) ([]storage.IncidentRecord, error)
}

type Options struct {
	Enabled         bool
	MaxIncidents    int
	IncludeResolved bool
	Now             func() time.Time
	SchedulerStatus func() agent.SchedulerStatus
}

type Summary struct {
	GeneratedAt                time.Time             `json:"generated_at"`
	StorageAvailable           bool                  `json:"storage_available"`
	SchemaVersion              int                   `json:"schema_version"`
	OpenIncidentsBySeverity    map[string]int64      `json:"open_incidents_by_severity"`
	IncidentStatusCounts       map[string]int64      `json:"incident_status_counts"`
	NotificationDeliveryCounts map[string]int64      `json:"notification_delivery_counts"`
	Scheduler                  agent.SchedulerStatus `json:"scheduler"`
	RecentResolvedIncidents    []IncidentSummary     `json:"recent_resolved_incidents,omitempty"`
	KnownLimitations           []string              `json:"known_limitations"`
	ErrorClass                 string                `json:"error_class,omitempty"`
	ErrorSummary               string                `json:"error_summary,omitempty"`
}

type IncidentSummary struct {
	ID              string     `json:"id"`
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
	UpdatedAt       time.Time  `json:"updated_at"`
}

func Generate(ctx context.Context, store Store, opts Options) (Summary, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.MaxIncidents <= 0 || opts.MaxIncidents > 1000 {
		opts.MaxIncidents = 100
	}
	summary := Summary{
		GeneratedAt:                opts.Now().UTC(),
		OpenIncidentsBySeverity:    map[string]int64{},
		IncidentStatusCounts:       map[string]int64{},
		NotificationDeliveryCounts: map[string]int64{},
		KnownLimitations: []string{
			"production scheduler is disabled unless explicitly configured",
			"report delivery is not implemented",
			"dashboard and remediation are not implemented",
		},
	}
	if opts.SchedulerStatus != nil {
		summary.Scheduler = agent.SafeSchedulerStatus(opts.SchedulerStatus())
	}
	if store == nil {
		summary.ErrorClass = "storage"
		summary.ErrorSummary = "storage is unavailable"
		return summary, nil
	}
	if err := store.Ping(ctx); err != nil {
		summary.ErrorClass = "storage"
		summary.ErrorSummary = redaction.Redact(err.Error())
		return summary, nil
	}
	summary.StorageAvailable = true
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return summary, err
	}
	summary.SchemaVersion = version
	statusCounts, err := store.IncidentStatusCounts(ctx)
	if err != nil {
		return summary, err
	}
	summary.IncidentStatusCounts = redactCountMap(statusCounts)
	deliveries, err := store.NotificationDeliveryStatusCounts(ctx)
	if err != nil {
		return summary, err
	}
	summary.NotificationDeliveryCounts = redactCountMap(deliveries)
	incidents, err := store.ListRecentIncidents(ctx, opts.MaxIncidents)
	if err != nil {
		return summary, err
	}
	for _, incident := range incidents {
		if incident.Status == "open" {
			summary.OpenIncidentsBySeverity[redaction.Redact(incident.Severity)]++
			continue
		}
		if opts.IncludeResolved && incident.Status == "resolved" {
			summary.RecentResolvedIncidents = append(summary.RecentResolvedIncidents, safeIncidentSummary(incident))
		}
	}
	sort.Slice(summary.RecentResolvedIncidents, func(i, j int) bool {
		return summary.RecentResolvedIncidents[i].UpdatedAt.After(summary.RecentResolvedIncidents[j].UpdatedAt)
	})
	return summary, nil
}

func safeIncidentSummary(record storage.IncidentRecord) IncidentSummary {
	return IncidentSummary{
		ID:              redaction.Redact(record.ID),
		NodeID:          redaction.Redact(record.NodeID),
		Type:            redaction.Redact(record.Type),
		Target:          redaction.Redact(record.Target),
		Condition:       redaction.Redact(record.Condition),
		Severity:        redaction.Redact(record.Severity),
		Status:          redaction.Redact(record.Status),
		Summary:         redaction.Redact(record.Summary),
		FirstSeen:       record.FirstSeen.UTC(),
		LastSeen:        record.LastSeen.UTC(),
		OccurrenceCount: record.OccurrenceCount,
		LastTransition:  utcPtr(record.LastTransition),
		ResolvedAt:      utcPtr(record.ResolvedAt),
		UpdatedAt:       record.UpdatedAt.UTC(),
	}
}

func redactCountMap(input map[string]int64) map[string]int64 {
	output := make(map[string]int64, len(input))
	for key, value := range input {
		output[redaction.Redact(key)] = value
	}
	return output
}

func utcPtr(input *time.Time) *time.Time {
	if input == nil {
		return nil
	}
	t := input.UTC()
	return &t
}
