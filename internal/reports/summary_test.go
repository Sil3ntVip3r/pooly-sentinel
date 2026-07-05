package reports

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

func TestGenerateSummaryFromStorage(t *testing.T) {
	store := openReportStore(t)
	now := fixedReportTime()
	if err := store.UpsertIncident(context.Background(), storage.IncidentRecord{
		ID:              "inc-open",
		Fingerprint:     "node:rule:system:open",
		NodeID:          "node",
		Type:            "rule",
		Target:          "system",
		Condition:       "cpu",
		Severity:        "critical",
		Status:          "open",
		Summary:         "open summary",
		FirstSeen:       now.Add(-time.Hour),
		LastSeen:        now,
		OccurrenceCount: 2,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("upsert open incident: %v", err)
	}
	if err := store.UpsertIncident(context.Background(), storage.IncidentRecord{
		ID:              "inc-resolved",
		Fingerprint:     "node:rule:system:resolved",
		NodeID:          "node",
		Type:            "rule",
		Target:          "system",
		Condition:       "memory",
		Severity:        "none",
		Status:          "resolved",
		Summary:         "token=supersecret",
		FirstSeen:       now.Add(-2 * time.Hour),
		LastSeen:        now.Add(-time.Hour),
		OccurrenceCount: 1,
		ResolvedAt:      &now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("upsert resolved incident: %v", err)
	}
	if err := store.InsertNotificationDelivery(context.Background(), storage.NotificationDeliveryRecord{
		ID:          "del-1",
		IncidentID:  "inc-open",
		Receiver:    "local-webhook",
		CostClass:   "free_core",
		Status:      "failed",
		Attempt:     1,
		AttemptedAt: now,
	}); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
	summary, err := Generate(context.Background(), store, Options{
		Enabled:         true,
		MaxIncidents:    100,
		IncludeResolved: true,
		Now:             fixedReportTime,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !summary.StorageAvailable || summary.SchemaVersion == 0 {
		t.Fatalf("storage summary = %+v", summary)
	}
	if summary.OpenIncidentsBySeverity["critical"] != 1 {
		t.Fatalf("open by severity = %+v", summary.OpenIncidentsBySeverity)
	}
	if summary.NotificationDeliveryCounts["failed"] != 1 {
		t.Fatalf("delivery counts = %+v", summary.NotificationDeliveryCounts)
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if strings.Contains(string(data), "supersecret") {
		t.Fatalf("summary leaked secret: %s", data)
	}
}

func openReportStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(context.Background(), storage.SQLiteOptions{
		Path:             filepath.Join(t.TempDir(), "state.db"),
		CreateParentDirs: true,
		BusyTimeout:      time.Second,
		WAL:              true,
		Synchronous:      "NORMAL",
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func fixedReportTime() time.Time {
	return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
}
