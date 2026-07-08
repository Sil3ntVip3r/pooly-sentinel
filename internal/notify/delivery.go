package notify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type Service struct {
	Enabled      bool
	DryRun       bool
	Store        Store
	Receivers    []Receiver
	Clock        Clock
	EvidenceRoot string
}

func NewService(opts Options, store Store, receivers []Receiver) Service {
	return Service{
		Enabled:      opts.Enabled,
		DryRun:       opts.DryRun,
		Store:        store,
		Receivers:    receivers,
		Clock:        RealClock{},
		EvidenceRoot: opts.EvidenceRoot,
	}
}

func (s Service) DeliverTransitions(ctx context.Context, transitions []incidents.Transition) (Report, error) {
	if ctx == nil {
		return Report{}, fmt.Errorf("context is nil")
	}
	var report Report
	for _, transition := range transitions {
		event, ok := EventFromTransition(transition)
		if !ok {
			continue
		}
		if s.Store == nil {
			return report, fmt.Errorf("notification store is nil")
		}
		incident, err := s.Store.GetIncident(ctx, transition.IncidentID)
		if err != nil {
			return report, err
		}
		partial, err := s.DeliverIncident(ctx, incident, event)
		report.add(partial)
		if err != nil {
			return report, err
		}
	}
	return report, nil
}

func (s Service) DeliverIncident(ctx context.Context, incident storage.IncidentRecord, event Event) (Report, error) {
	if ctx == nil {
		return Report{}, fmt.Errorf("context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	var report Report
	if !s.Enabled && !s.DryRun {
		return Report{Results: []DeliveryResult{{IncidentID: incident.ID, Event: event, Status: StatusSkipped, Summary: "notifications disabled"}}, Skipped: 1}, nil
	}
	if len(s.Receivers) == 0 {
		return Report{Results: []DeliveryResult{{IncidentID: incident.ID, Event: event, Status: StatusSkipped, Summary: "no receivers configured"}}, Skipped: 1}, nil
	}
	payload := RenderPayloadWithEvidenceRoot(incident, event, s.EvidenceRoot)
	severity := incidents.Severity(incident.Severity)
	if event == EventResolved {
		severity = incidents.SeverityNone
	}
	for _, receiver := range s.Receivers {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		result, err := s.deliverToReceiver(ctx, receiver, incident, payload, event, severity)
		report.addResult(result)
		if err != nil {
			return report, err
		}
	}
	return report, nil
}

func (s Service) deliverToReceiver(ctx context.Context, receiver Receiver, incident storage.IncidentRecord, payload Payload, event Event, severity incidents.Severity) (DeliveryResult, error) {
	result := DeliveryResult{
		IncidentID: incident.ID,
		ReceiverID: receiver.ID(),
		Event:      event,
	}
	if !receiver.Enabled() && !s.DryRun {
		result.Status = StatusSkipped
		result.Summary = "receiver disabled"
		return result, nil
	}
	if !receiver.Matches(event, severity) {
		result.Status = StatusSkipped
		result.Summary = "receiver filters did not match"
		return result, nil
	}
	if s.DryRun {
		result.Status = StatusDryRun
		result.Summary = redaction.Redact("dry-run payload rendered for " + receiver.Summary())
		return result, nil
	}
	if s.Store == nil {
		return result, fmt.Errorf("notification store is nil")
	}
	deliveries, err := s.Store.ListNotificationDeliveries(ctx, incident.ID)
	if err != nil {
		return result, err
	}
	key := deliveryKey(incident, receiver.ID(), event)
	attempt, delivered := nextAttempt(deliveries, key)
	result.Attempt = attempt
	if delivered {
		result.Status = StatusSkipped
		result.Summary = "duplicate notification suppressed"
		return result, nil
	}

	outcome := receiver.Deliver(ctx, payload)
	now := s.now()
	record := storage.NotificationDeliveryRecord{
		ID:          deliveryID(key, attempt),
		IncidentID:  incident.ID,
		Receiver:    receiver.ID(),
		CostClass:   receiver.CostClass(),
		Attempt:     attempt,
		AttemptedAt: now,
	}
	if outcome.Success {
		record.Status = string(StatusDelivered)
		record.DeliveredAt = &now
		result.Status = StatusDelivered
		result.Summary = safeResultSummary(outcome.Summary)
		result.Attempt = attempt
		err := s.Store.NotificationDeliveryTransaction(ctx, func(tx storage.NotificationDeliveryTransaction) error {
			if err := tx.InsertNotificationDelivery(ctx, record); err != nil {
				return err
			}
			return tx.UpdateIncidentLastAlerted(ctx, incident.ID, now)
		})
		if err != nil {
			return result, err
		}
		return result, nil
	}

	record.Status = string(StatusFailed)
	record.ErrorClass = safeResultSummary(outcome.ErrorClass)
	record.ErrorSummary = safeResultSummary(outcome.Summary)
	result.Status = StatusFailed
	result.ErrorClass = record.ErrorClass
	result.Summary = record.ErrorSummary
	result.Attempt = attempt
	err = s.Store.NotificationDeliveryTransaction(ctx, func(tx storage.NotificationDeliveryTransaction) error {
		return tx.InsertNotificationDelivery(ctx, record)
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func EventFromTransition(transition incidents.Transition) (Event, bool) {
	switch transition.Action {
	case incidents.ActionOpened:
		return EventOpened, true
	case incidents.ActionReopened:
		return EventOpened, true
	case incidents.ActionEscalated:
		return EventEscalated, true
	case incidents.ActionResolved:
		return EventResolved, true
	default:
		return "", false
	}
}

func EventFromIncident(incident storage.IncidentRecord) Event {
	if incident.Status == string(incidents.StatusResolved) {
		return EventResolved
	}
	return EventOpened
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now().UTC()
	}
	return time.Now().UTC()
}

func safeResultSummary(summary string) string {
	value := redaction.Redact(summary)
	if value == "" {
		return "notification delivery result"
	}
	if len(value) > 300 {
		return value[:300]
	}
	return value
}

func (r *Report) add(other Report) {
	for _, result := range other.Results {
		r.addResult(result)
	}
}

func (r *Report) addResult(result DeliveryResult) {
	r.Results = append(r.Results, result)
	switch result.Status {
	case StatusDelivered:
		r.Delivered++
	case StatusFailed:
		r.Failed++
	case StatusDryRun:
		r.DryRun++
	case StatusSkipped:
		r.Skipped++
	}
}

func Failed(report Report) bool {
	return report.Failed > 0
}

var ErrDeliveryFailed = errors.New("notification delivery failed")
