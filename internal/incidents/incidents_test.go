package incidents

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type memoryIncidentStore struct {
	records map[string]storage.IncidentRecord
	fail    bool
}

func newMemoryIncidentStore() *memoryIncidentStore {
	return &memoryIncidentStore{records: map[string]storage.IncidentRecord{}}
}

func (s *memoryIncidentStore) GetIncidentByFingerprint(ctx context.Context, fingerprint string) (storage.IncidentRecord, error) {
	if err := ctx.Err(); err != nil {
		return storage.IncidentRecord{}, err
	}
	if s.fail {
		return storage.IncidentRecord{}, errors.New("store failed")
	}
	record, ok := s.records[fingerprint]
	if !ok {
		return storage.IncidentRecord{}, storage.ErrNotFound
	}
	return record, nil
}

func (s *memoryIncidentStore) UpsertIncident(ctx context.Context, record storage.IncidentRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.fail {
		return errors.New("store failed")
	}
	s.records[record.Fingerprint] = record
	return nil
}

func TestFingerprintStabilityAndRejection(t *testing.T) {
	fp, err := Fingerprint("Node-001", "Systemd", "SSH.Service", "pooly_systemd_unit_failed")
	if err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}
	if fp != "node-001:systemd:ssh.service:pooly_systemd_unit_failed" {
		t.Fatalf("fingerprint = %q", fp)
	}
	if _, err := Fingerprint("node", "systemd", "", "condition"); err == nil {
		t.Fatal("Fingerprint() empty target error = nil")
	}
	if _, err := Fingerprint("node", "systemd", "secret token=abc", "condition"); err == nil {
		t.Fatal("Fingerprint() unsafe input error = nil")
	}
}

func TestIncidentDedupEscalateResolveAndReopen(t *testing.T) {
	store := newMemoryIncidentStore()
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	engine := NewEngine(func() time.Time { return now })
	candidate := Candidate{
		NodeID:     "node-001",
		Type:       "systemd",
		Target:     "ssh.service",
		Condition:  "pooly_systemd_unit_failed",
		Severity:   SeverityWarning,
		Active:     true,
		Summary:    "service failed",
		ObservedAt: now,
	}
	opened, err := engine.Apply(context.Background(), store, candidate)
	if err != nil {
		t.Fatalf("Apply(open) error = %v", err)
	}
	if opened.Action != ActionOpened {
		t.Fatalf("open action = %s", opened.Action)
	}
	updated, err := engine.Apply(context.Background(), store, candidate)
	if err != nil {
		t.Fatalf("Apply(update) error = %v", err)
	}
	if updated.Action != ActionUpdated {
		t.Fatalf("update action = %s", updated.Action)
	}
	candidate.Severity = SeverityFailure
	escalated, err := engine.Apply(context.Background(), store, candidate)
	if err != nil {
		t.Fatalf("Apply(escalate) error = %v", err)
	}
	if escalated.Action != ActionEscalated || escalated.Severity != SeverityFailure {
		t.Fatalf("escalated = %+v", escalated)
	}
	candidate.Active = false
	candidate.Severity = SeverityNone
	resolved, err := engine.Apply(context.Background(), store, candidate)
	if err != nil {
		t.Fatalf("Apply(resolve) error = %v", err)
	}
	if resolved.Action != ActionResolved || resolved.Status != StatusResolved {
		t.Fatalf("resolved = %+v", resolved)
	}
	candidate.Active = true
	candidate.Severity = SeverityWarning
	reopened, err := engine.Apply(context.Background(), store, candidate)
	if err != nil {
		t.Fatalf("Apply(reopen) error = %v", err)
	}
	if reopened.Action != ActionReopened || reopened.IncidentID != opened.IncidentID {
		t.Fatalf("reopened = %+v opened=%+v", reopened, opened)
	}
	record := store.records[opened.Fingerprint]
	if record.OccurrenceCount != 4 {
		t.Fatalf("occurrence count = %d, want 4", record.OccurrenceCount)
	}
}

func TestIncidentContextCancellationAndStoreFailure(t *testing.T) {
	store := newMemoryIncidentStore()
	engine := NewEngine(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := engine.Apply(ctx, store, Candidate{NodeID: "node", Type: "rule", Target: "system", Condition: "condition", Severity: SeverityWarning, Active: true})
	if err == nil {
		t.Fatal("Apply() canceled error = nil")
	}
	store.fail = true
	_, err = engine.Apply(context.Background(), store, Candidate{NodeID: "node", Type: "rule", Target: "system", Condition: "condition", Severity: SeverityWarning, Active: true})
	if err == nil {
		t.Fatal("Apply() store failure error = nil")
	}
}
