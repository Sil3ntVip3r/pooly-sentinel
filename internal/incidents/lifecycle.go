package incidents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type Store interface {
	GetIncidentByFingerprint(ctx context.Context, fingerprint string) (storage.IncidentRecord, error)
	UpsertIncident(ctx context.Context, record storage.IncidentRecord) error
}

type Engine struct {
	Clock func() time.Time
}

func NewEngine(clock func() time.Time) Engine {
	return Engine{Clock: clock}
}

func (e Engine) Apply(ctx context.Context, store Store, candidate Candidate) (Transition, error) {
	if ctx == nil {
		return Transition{}, fmt.Errorf("context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Transition{}, err
	}
	if store == nil {
		return Transition{}, fmt.Errorf("incident store is nil")
	}
	now := candidate.ObservedAt.UTC()
	if now.IsZero() {
		now = e.now()
	}
	fingerprint, err := Fingerprint(candidate.NodeID, candidate.Type, candidate.Target, candidate.Condition)
	if err != nil {
		return Transition{}, err
	}
	incidentID, err := IncidentIDForFingerprint(fingerprint)
	if err != nil {
		return Transition{}, err
	}
	existing, err := store.GetIncidentByFingerprint(ctx, fingerprint)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return Transition{}, err
	}
	if !candidate.Active {
		if errors.Is(err, storage.ErrNotFound) {
			return Transition{Action: ActionNone, IncidentID: incidentID, Fingerprint: fingerprint, Severity: SeverityNone, Status: StatusResolved}, nil
		}
		return e.resolve(ctx, store, existing, candidate, now)
	}
	if candidate.Severity == "" || candidate.Severity == SeverityNone {
		return Transition{}, fmt.Errorf("active incident candidate requires severity")
	}
	if errors.Is(err, storage.ErrNotFound) {
		record := storage.IncidentRecord{
			ID:              incidentID,
			Fingerprint:     fingerprint,
			NodeID:          normalizeForRecord(candidate.NodeID),
			Type:            normalizeForRecord(candidate.Type),
			Target:          normalizeForRecord(candidate.Target),
			Condition:       normalizeForRecord(candidate.Condition),
			Severity:        string(candidate.Severity),
			Status:          string(StatusOpen),
			Summary:         safeSummary(candidate.Summary),
			FirstSeen:       now,
			LastSeen:        now,
			OccurrenceCount: 1,
			EvidencePath:    redaction.Redact(candidate.EvidencePath),
			LastTransition:  &now,
			UpdatedAt:       now,
		}
		if err := store.UpsertIncident(ctx, record); err != nil {
			return Transition{}, err
		}
		return transition(ActionOpened, record), nil
	}
	return e.updateActive(ctx, store, existing, candidate, now)
}

func (e Engine) updateActive(ctx context.Context, store Store, record storage.IncidentRecord, candidate Candidate, now time.Time) (Transition, error) {
	action := ActionUpdated
	if record.Status == string(StatusResolved) {
		action = ActionReopened
		record.ResolvedAt = nil
		record.LastTransition = &now
	} else if severityRank(candidate.Severity) > severityRank(Severity(record.Severity)) {
		action = ActionEscalated
		record.LastTransition = &now
	} else if Severity(record.Severity) != candidate.Severity {
		record.LastTransition = &now
	}
	record.Status = string(StatusOpen)
	record.Severity = string(candidate.Severity)
	record.Summary = safeSummary(candidate.Summary)
	record.LastSeen = now
	record.OccurrenceCount++
	record.UpdatedAt = now
	if record.Fingerprint == "" {
		fingerprint, err := Fingerprint(candidate.NodeID, candidate.Type, candidate.Target, candidate.Condition)
		if err != nil {
			return Transition{}, err
		}
		record.Fingerprint = fingerprint
	}
	if record.ID == "" {
		incidentID, err := IncidentIDForFingerprint(record.Fingerprint)
		if err != nil {
			return Transition{}, err
		}
		record.ID = incidentID
	}
	if record.EvidencePath == "" {
		record.EvidencePath = redaction.Redact(candidate.EvidencePath)
	}
	if err := store.UpsertIncident(ctx, record); err != nil {
		return Transition{}, err
	}
	return transition(action, record), nil
}

func (e Engine) resolve(ctx context.Context, store Store, record storage.IncidentRecord, candidate Candidate, now time.Time) (Transition, error) {
	if record.Status == string(StatusResolved) {
		record.LastSeen = now
		record.UpdatedAt = now
		record.Summary = safeSummary(candidate.Summary)
		if err := store.UpsertIncident(ctx, record); err != nil {
			return Transition{}, err
		}
		return transition(ActionUpdated, record), nil
	}
	record.Status = string(StatusResolved)
	record.Severity = string(SeverityNone)
	record.Summary = safeSummary(candidate.Summary)
	record.LastSeen = now
	record.ResolvedAt = &now
	record.LastTransition = &now
	record.UpdatedAt = now
	if err := store.UpsertIncident(ctx, record); err != nil {
		return Transition{}, err
	}
	return transition(ActionResolved, record), nil
}

func (e Engine) now() time.Time {
	if e.Clock != nil {
		return e.Clock().UTC()
	}
	return time.Now().UTC()
}

func transition(action Action, record storage.IncidentRecord) Transition {
	return Transition{
		Action:      action,
		IncidentID:  record.ID,
		Fingerprint: record.Fingerprint,
		Severity:    Severity(record.Severity),
		Status:      Status(record.Status),
		Summary:     record.Summary,
	}
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 3
	case SeverityFailure:
		return 2
	case SeverityWarning:
		return 1
	default:
		return 0
	}
}

func safeSummary(summary string) string {
	value := redaction.Redact(summary)
	if len(value) > 240 {
		return value[:240]
	}
	if value == "" {
		return "rule condition observed"
	}
	return value
}

func normalizeForRecord(value string) string {
	normalized, err := normalizeFingerprintComponent(value)
	if err != nil {
		return redaction.Redact(value)
	}
	return normalized
}
