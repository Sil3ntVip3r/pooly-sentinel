package api

import (
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type StatusResponse struct {
	ServiceStatus             string           `json:"service_status"`
	CurrentTime               time.Time        `json:"current_time"`
	StorageAvailable          bool             `json:"storage_available"`
	SchemaVersion             int              `json:"schema_version"`
	OpenIncidentCount         int64            `json:"open_incident_count"`
	ResolvedIncidentCount     int64            `json:"resolved_incident_count"`
	NotificationDeliveryCount int64            `json:"notification_delivery_count"`
	IncidentCounts            map[string]int64 `json:"incident_counts"`
	DeliveryCounts            map[string]int64 `json:"delivery_counts"`
	Readiness                 bool             `json:"readiness"`
	ErrorClass                string           `json:"error_class,omitempty"`
	ErrorSummary              string           `json:"error_summary,omitempty"`
}

type IncidentResponse struct {
	ID              string     `json:"id"`
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
	LastAlerted     *time.Time `json:"last_alerted,omitempty"`
	OccurrenceCount int64      `json:"occurrence_count"`
	EvidencePath    string     `json:"evidence_path,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	LastTransition  *time.Time `json:"last_transition,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type DeliveryResponse struct {
	ID           string     `json:"id"`
	IncidentID   string     `json:"incident_id"`
	Receiver     string     `json:"receiver"`
	CostClass    string     `json:"cost_class"`
	Status       string     `json:"status"`
	Attempt      int        `json:"attempt"`
	AttemptedAt  time.Time  `json:"attempted_at"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty"`
	ErrorClass   string     `json:"error_class,omitempty"`
	ErrorSummary string     `json:"error_summary,omitempty"`
}

func safeIncident(record storage.IncidentRecord) IncidentResponse {
	return IncidentResponse{
		ID:              redaction.Redact(record.ID),
		Fingerprint:     redaction.Redact(record.Fingerprint),
		NodeID:          redaction.Redact(record.NodeID),
		Type:            redaction.Redact(record.Type),
		Target:          redaction.Redact(record.Target),
		Condition:       redaction.Redact(record.Condition),
		Severity:        redaction.Redact(record.Severity),
		Status:          redaction.Redact(record.Status),
		Summary:         redaction.Redact(record.Summary),
		FirstSeen:       record.FirstSeen.UTC(),
		LastSeen:        record.LastSeen.UTC(),
		LastAlerted:     utcPtr(record.LastAlerted),
		OccurrenceCount: record.OccurrenceCount,
		EvidencePath:    safeEvidencePath(record.EvidencePath),
		ResolvedAt:      utcPtr(record.ResolvedAt),
		LastTransition:  utcPtr(record.LastTransition),
		UpdatedAt:       record.UpdatedAt.UTC(),
	}
}

func safeDelivery(record storage.NotificationDeliveryRecord) DeliveryResponse {
	return DeliveryResponse{
		ID:           redaction.Redact(record.ID),
		IncidentID:   redaction.Redact(record.IncidentID),
		Receiver:     redaction.Redact(record.Receiver),
		CostClass:    redaction.Redact(record.CostClass),
		Status:       redaction.Redact(record.Status),
		Attempt:      record.Attempt,
		AttemptedAt:  record.AttemptedAt.UTC(),
		DeliveredAt:  utcPtr(record.DeliveredAt),
		ErrorClass:   redaction.Redact(record.ErrorClass),
		ErrorSummary: redaction.Redact(record.ErrorSummary),
	}
}

func utcPtr(input *time.Time) *time.Time {
	if input == nil {
		return nil
	}
	t := input.UTC()
	return &t
}
