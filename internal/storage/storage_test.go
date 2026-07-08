package storage

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDatabaseCreationAndFreshMigration(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state", "state.db")
	store := openTestStore(t, dbPath)
	defer store.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database was not created: %v", err)
	}
	assertRestrictiveFileMode(t, dbPath)
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion() error = %v", err)
	}
	if version != LatestSchemaVersion() {
		t.Fatalf("schema version = %d, want %d", version, LatestSchemaVersion())
	}
}

func TestReopeningAlreadyMigratedDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	first := openTestStore(t, dbPath)
	if err := first.UpsertMetadata(context.Background(), MetadataRecord{Key: "agent", Value: "ok"}); err != nil {
		t.Fatalf("UpsertMetadata() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := openTestStore(t, dbPath)
	defer second.Close()
	got, err := second.GetMetadata(context.Background(), "agent")
	if err != nil {
		t.Fatalf("GetMetadata() after reopen error = %v", err)
	}
	if got.Value != "ok" {
		t.Fatalf("metadata value = %q, want ok", got.Value)
	}
}

func TestRepeatedInitialization(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	for i := 0; i < 3; i++ {
		store := openTestStore(t, dbPath)
		if err := store.Close(); err != nil {
			t.Fatalf("Close() pass %d error = %v", i, err)
		}
	}
}

func TestMigrationOrdering(t *testing.T) {
	migrations := []Migration{
		{Version: 2, Name: "two", Statements: []string{`CREATE TABLE two (id INTEGER PRIMARY KEY)`}},
		{Version: 1, Name: "one", Statements: []string{`CREATE TABLE one (id INTEGER PRIMARY KEY)`}},
	}
	db := openRawTestDB(t)
	defer db.Close()
	if err := runMigrations(context.Background(), db, migrations); err != nil {
		t.Fatalf("runMigrations() error = %v", err)
	}
	version, err := currentSchemaVersion(context.Background(), db)
	if err != nil {
		t.Fatalf("currentSchemaVersion() error = %v", err)
	}
	if version != 2 {
		t.Fatalf("schema version = %d, want 2", version)
	}
}

func TestMigrationRollbackOnFailure(t *testing.T) {
	db := openRawTestDB(t)
	defer db.Close()
	migrations := []Migration{
		{Version: 1, Name: "bad", Statements: []string{
			`CREATE TABLE rollback_probe (id INTEGER PRIMARY KEY)`,
			`INSERT INTO missing_table(value) VALUES ('boom')`,
		}},
	}
	err := runMigrations(context.Background(), db, migrations)
	if err == nil {
		t.Fatal("runMigrations() error = nil, want failure")
	}
	var count int
	queryErr := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if queryErr != nil {
		t.Fatalf("query schema_migrations: %v", queryErr)
	}
	if count != 0 {
		t.Fatalf("migration record count = %d, want 0", count)
	}
	var tableName string
	err = db.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'rollback_probe'`).Scan(&tableName)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rollback_probe table should not exist, err=%v name=%q", err, tableName)
	}
}

func TestUnsupportedFutureSchemaVersion(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store := openTestStore(t, dbPath)
	db, err := store.database()
	if err != nil {
		t.Fatalf("database() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`, LatestSchemaVersion()+100, "future", formatTime(nowUTC())); err != nil {
		t.Fatalf("insert future migration: %v", err)
	}
	_ = store.Close()

	_, err = Open(ctx, testSQLiteOptions(dbPath))
	if err == nil {
		t.Fatal("Open() error = nil, want future schema error")
	}
	if !errors.Is(err, ErrFutureSchema) {
		t.Fatalf("Open() error = %v, want ErrFutureSchema", err)
	}
}

func TestMetadataReadWriteAndNotFound(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer store.Close()
	ctx := context.Background()
	if err := store.UpsertMetadata(ctx, MetadataRecord{Key: "k", Value: "v"}); err != nil {
		t.Fatalf("UpsertMetadata() error = %v", err)
	}
	got, err := store.GetMetadata(ctx, "k")
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	if got.Value != "v" || got.UpdatedAt.IsZero() {
		t.Fatalf("metadata = %+v, want value and timestamp", got)
	}
	records, err := store.ListMetadata(ctx)
	if err != nil {
		t.Fatalf("ListMetadata() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("metadata count = %d, want 1", len(records))
	}
	if err := store.DeleteMetadata(ctx, "k"); err != nil {
		t.Fatalf("DeleteMetadata() error = %v", err)
	}
	_, err = store.GetMetadata(ctx, "k")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetMetadata() error = %v, want ErrNotFound", err)
	}
}

func TestCollectorStateUpsert(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	record := CollectorStateRecord{
		Collector:        "systemd",
		Target:           "ssh.service",
		Status:           "ok",
		StateJSON:        `{"ok":true}`,
		LastAttemptAt:    &now,
		LastSuccessAt:    &now,
		LastErrorClass:   "none",
		LastErrorSummary: "none",
	}
	if err := store.UpsertCollectorState(ctx, record); err != nil {
		t.Fatalf("UpsertCollectorState() error = %v", err)
	}
	got, err := store.GetCollectorState(ctx, "systemd", "ssh.service")
	if err != nil {
		t.Fatalf("GetCollectorState() error = %v", err)
	}
	if got.StateJSON != record.StateJSON || got.LastAttemptAt == nil || got.LastSuccessAt == nil {
		t.Fatalf("collector state = %+v", got)
	}
}

func TestIncidentAndNotificationPersistence(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	incident := IncidentRecord{
		ID:              "001:service:ssh.service:failed",
		Fingerprint:     "001:service:ssh.service:failed",
		NodeID:          "001",
		Type:            "service",
		Target:          "ssh.service",
		Condition:       "failed",
		Severity:        "FAIL",
		Status:          "open",
		Summary:         "service failed",
		FirstSeen:       now,
		LastSeen:        now,
		OccurrenceCount: 1,
		EvidencePath:    "incidents/open/id/evidence.json",
	}
	if err := store.UpsertIncident(ctx, incident); err != nil {
		t.Fatalf("UpsertIncident() error = %v", err)
	}
	got, err := store.GetIncident(ctx, incident.ID)
	if err != nil {
		t.Fatalf("GetIncident() error = %v", err)
	}
	if got.ID != incident.ID || got.EvidencePath != incident.EvidencePath {
		t.Fatalf("incident = %+v", got)
	}
	byFingerprint, err := store.GetIncidentByFingerprint(ctx, incident.Fingerprint)
	if err != nil {
		t.Fatalf("GetIncidentByFingerprint() error = %v", err)
	}
	if byFingerprint.ID != incident.ID {
		t.Fatalf("incident by fingerprint = %+v", byFingerprint)
	}
	delivery := NotificationDeliveryRecord{
		ID:          "delivery-1",
		IncidentID:  incident.ID,
		Receiver:    "local_file",
		CostClass:   "free_core",
		Status:      "queued",
		Attempt:     1,
		AttemptedAt: now,
	}
	if err := store.InsertNotificationDelivery(ctx, delivery); err != nil {
		t.Fatalf("InsertNotificationDelivery() error = %v", err)
	}
	deliveries, err := store.ListNotificationDeliveries(ctx, incident.ID)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].ID != delivery.ID {
		t.Fatalf("deliveries = %+v", deliveries)
	}
}

func TestRuleEvaluationStatePersistence(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	record := RuleEvaluationStateRecord{
		RuleID:            "memory-low",
		Target:            "system",
		State:             "PENDING_WARN",
		Severity:          "none",
		ConditionMetSince: &now,
		LastEvaluatedAt:   now,
		LastResultSummary: "pending",
		PendingSeverity:   "warning",
	}
	if err := store.UpsertRuleEvaluationState(ctx, record); err != nil {
		t.Fatalf("UpsertRuleEvaluationState() error = %v", err)
	}
	got, err := store.GetRuleEvaluationState(ctx, record.RuleID, record.Target)
	if err != nil {
		t.Fatalf("GetRuleEvaluationState() error = %v", err)
	}
	if got.State != record.State || got.ConditionMetSince == nil || got.PendingSeverity != "warning" {
		t.Fatalf("rule state = %+v", got)
	}
	list, err := store.ListRuleEvaluationState(ctx)
	if err != nil {
		t.Fatalf("ListRuleEvaluationState() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("rule states = %d, want 1", len(list))
	}
}

func TestRuleEvaluationTransactionRollback(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	err := store.RuleEvaluationTransaction(ctx, func(tx RuleEvaluationTransaction) error {
		if err := tx.UpsertRuleEvaluationState(ctx, RuleEvaluationStateRecord{
			RuleID:            "cpu-high",
			Target:            "system",
			State:             "WARN",
			Severity:          "warning",
			LastEvaluatedAt:   now,
			LastResultSummary: "warning",
		}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("RuleEvaluationTransaction() error = nil")
	}
	_, err = store.GetRuleEvaluationState(ctx, "cpu-high", "system")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRuleEvaluationState() error = %v, want ErrNotFound", err)
	}
}

func TestNotificationDeliveryTransactionUpdatesLastAlertedAndRollsBack(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	incident := IncidentRecord{
		ID:              "inc-notify",
		Fingerprint:     "node:rule:system:condition",
		NodeID:          "node",
		Type:            "rule",
		Target:          "system",
		Condition:       "condition",
		Severity:        "warning",
		Status:          "open",
		Summary:         "summary",
		FirstSeen:       now,
		LastSeen:        now,
		OccurrenceCount: 1,
	}
	if err := store.UpsertIncident(ctx, incident); err != nil {
		t.Fatalf("UpsertIncident() error = %v", err)
	}
	alertedAt := now.Add(time.Minute)
	err := store.NotificationDeliveryTransaction(ctx, func(tx NotificationDeliveryTransaction) error {
		if err := tx.InsertNotificationDelivery(ctx, NotificationDeliveryRecord{
			ID:          "delivered-1",
			IncidentID:  incident.ID,
			Receiver:    "web",
			CostClass:   "free_external",
			Status:      "delivered",
			Attempt:     1,
			AttemptedAt: alertedAt,
			DeliveredAt: &alertedAt,
		}); err != nil {
			return err
		}
		return tx.UpdateIncidentLastAlerted(ctx, incident.ID, alertedAt)
	})
	if err != nil {
		t.Fatalf("NotificationDeliveryTransaction() error = %v", err)
	}
	got, err := store.GetIncident(ctx, incident.ID)
	if err != nil {
		t.Fatalf("GetIncident() error = %v", err)
	}
	if got.LastAlerted == nil || !got.LastAlerted.Equal(alertedAt) {
		t.Fatalf("LastAlerted = %v, want %v", got.LastAlerted, alertedAt)
	}
	err = store.NotificationDeliveryTransaction(ctx, func(tx NotificationDeliveryTransaction) error {
		if err := tx.InsertNotificationDelivery(ctx, NotificationDeliveryRecord{
			ID:          "rolled-back",
			IncidentID:  incident.ID,
			Receiver:    "web",
			CostClass:   "free_external",
			Status:      "delivered",
			Attempt:     2,
			AttemptedAt: alertedAt,
			DeliveredAt: &alertedAt,
		}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("NotificationDeliveryTransaction() rollback error = nil")
	}
	deliveries, err := store.ListNotificationDeliveries(ctx, incident.ID)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}
	for _, delivery := range deliveries {
		if delivery.ID == "rolled-back" {
			t.Fatalf("rolled back delivery persisted: %+v", deliveries)
		}
	}
}

func TestContextCancellationAndCloseBehavior(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "state.db"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := store.UpsertMetadata(ctx, MetadataRecord{Key: "k", Value: "v"})
	if err == nil {
		t.Fatal("UpsertMetadata() error = nil, want canceled context")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := store.Ping(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Ping() after close error = %v, want ErrClosed", err)
	}
}

func TestInvalidDatabasePath(t *testing.T) {
	_, err := Open(context.Background(), SQLiteOptions{
		Path:             filepath.Join(t.TempDir(), "missing", "state.db"),
		CreateParentDirs: false,
		BusyTimeout:      time.Second,
		WAL:              true,
	})
	if err == nil {
		t.Fatal("Open() error = nil, want invalid path error")
	}
}

func TestConcurrentRepositoryCallsRaceSafe(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer store.Close()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := "k" + string(rune('a'+i))
			if err := store.UpsertMetadata(ctx, MetadataRecord{Key: key, Value: "value"}); err != nil {
				t.Errorf("UpsertMetadata(%s) error = %v", key, err)
				return
			}
			if _, err := store.GetMetadata(ctx, key); err != nil {
				t.Errorf("GetMetadata(%s) error = %v", key, err)
			}
		}()
	}
	wg.Wait()
}

func TestCurrentStateAtomicWriteRedactsAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "metrics-current.json")
	err := WriteCurrentState(context.Background(), path, map[string]any{
		"node":     "001",
		"password": "super-secret",
		"url":      "https://example.test/?token=abc123",
	})
	if err != nil {
		t.Fatalf("WriteCurrentState() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "super-secret") || strings.Contains(string(data), "abc123") {
		t.Fatalf("current state leaked secret: %s", data)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("current state invalid JSON: %v", err)
	}
	assertRestrictiveFileMode(t, path)
}

func TestJSONLEventWriterValidityOversizeRedactionAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events", "warnfail.jsonl")
	writer, err := OpenEventWriter(context.Background(), EventWriterOptions{Path: path, MaxEventBytes: 512, SyncOnWrite: true})
	if err != nil {
		t.Fatalf("OpenEventWriter() error = %v", err)
	}
	if err := writer.Write(context.Background(), map[string]any{"kind": "warn", "api_key": "secret-key"}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Write(context.Background(), map[string]any{"payload": strings.Repeat("x", 1024)}); err == nil {
		t.Fatal("Write() oversized error = nil")
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := writer.Write(context.Background(), map[string]any{"kind": "after-close"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Write() after close error = %v, want ErrClosed", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open() event file error = %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
		var decoded map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &decoded); err != nil {
			t.Fatalf("line %d invalid JSON: %v", lines, err)
		}
		if strings.Contains(scanner.Text(), "secret-key") {
			t.Fatalf("event leaked secret: %s", scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan event file: %v", err)
	}
	if lines != 1 {
		t.Fatalf("jsonl lines = %d, want 1", lines)
	}
	assertRestrictiveFileMode(t, path)
}

func TestEvidenceWriterSanitizesRejectsTraversalAndRedacts(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	writer := NewEvidenceWriter(logDir)
	path, err := writer.WriteText(context.Background(), "001:ssh:password auth", "evidence.txt", "password=hunter2")
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}
	if !strings.Contains(path, filepath.Join("incidents", "open")) {
		t.Fatalf("evidence path = %q, want incidents/open", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "hunter2") {
		t.Fatalf("evidence leaked secret: %s", data)
	}
	assertRestrictiveFileMode(t, path)

	if _, err := writer.WriteText(context.Background(), "id", "../evil.txt", "nope"); err == nil {
		t.Fatal("WriteText() traversal error = nil")
	}
	if _, err := writer.WriteText(context.Background(), "id", filepath.Join(string(filepath.Separator), "evil.txt"), "nope"); err == nil {
		t.Fatal("WriteText() absolute filename error = nil")
	}
}

func TestEvidenceWriterJSONAndSymlinkDirectoryRejection(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(filepath.Join(root, "outside"), DirMode); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), logDir); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	writer := NewEvidenceWriter(logDir)
	_, err := writer.WriteJSON(context.Background(), "id", "evidence.json", map[string]any{"token": "abc123"})
	if err == nil {
		t.Fatal("WriteJSON() error = nil, want symlink rejection")
	}
}

func TestSQLiteAndJSONLRejectSymlinkFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.db")
	if err := os.WriteFile(target, []byte("not sqlite"), FileMode); err != nil {
		t.Fatalf("write target: %v", err)
	}
	dbLink := filepath.Join(root, "state.db")
	if err := os.Symlink(target, dbLink); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	if _, err := Open(context.Background(), testSQLiteOptions(dbLink)); err == nil {
		t.Fatal("Open() symlink database error = nil")
	}

	eventTarget := filepath.Join(root, "events-target.jsonl")
	if err := os.WriteFile(eventTarget, []byte(""), FileMode); err != nil {
		t.Fatalf("write event target: %v", err)
	}
	eventLink := filepath.Join(root, "events.jsonl")
	if err := os.Symlink(eventTarget, eventLink); err != nil {
		t.Fatalf("event symlink: %v", err)
	}
	if _, err := OpenEventWriter(context.Background(), EventWriterOptions{Path: eventLink}); err == nil {
		t.Fatal("OpenEventWriter() symlink event file error = nil")
	}
}

func TestAtomicWriteReturnsDirectorySyncError(t *testing.T) {
	oldSyncDir := syncDirFunc
	syncDirFunc = func(string) error { return errors.New("sync dir failed") }
	defer func() { syncDirFunc = oldSyncDir }()

	err := atomicWriteFile(context.Background(), filepath.Join(t.TempDir(), "state", "current.json"), []byte("{}\n"))
	if err == nil || !strings.Contains(err.Error(), "sync dir failed") {
		t.Fatalf("atomicWriteFile() error = %v, want sync dir failure", err)
	}
}

func TestSafeEvidencePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "logs")
	inside := filepath.Join(root, "incidents", "open", "id", "evidence.v1.json")
	cases := []struct {
		name string
		path string
		root string
		want string
	}{
		{name: "relative dotted filename", path: "incidents/open/id/evidence.v1.json", root: root, want: filepath.Join("incidents", "open", "id", "evidence.v1.json")},
		{name: "absolute inside root", path: inside, root: root, want: filepath.Join("incidents", "open", "id", "evidence.v1.json")},
		{name: "url rejected", path: "https://example.test/evidence.json", root: root},
		{name: "traversal rejected", path: "incidents/open/../secret.json", root: root},
		{name: "control rejected", path: "incidents/open/id/bad\nname.json", root: root},
		{name: "redacted rejected", path: "incidents/open/id/token=supersecret.json", root: root},
		{name: "absolute outside root rejected", path: filepath.Join(t.TempDir(), "other", "evidence.json"), root: root},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SafeEvidencePath(tc.path, tc.root); got != tc.want {
				t.Fatalf("SafeEvidencePath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDoctorUsesTemporaryDiagnostics(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	logDir := filepath.Join(t.TempDir(), "logs")
	checks := RunDoctor(context.Background(), DoctorOptions{
		StateDir:           stateDir,
		LogDir:             logDir,
		DatabaseFile:       "state.db",
		CurrentMetricsFile: "metrics-current.json",
		BusyTimeout:        time.Second,
		WAL:                true,
	})
	if DoctorFailed(checks) {
		t.Fatalf("doctor failed: %+v", checks)
	}
	if _, err := os.Stat(filepath.Join(stateDir, ".pooly-doctor-current-state.json")); !os.IsNotExist(err) {
		t.Fatalf("current-state diagnostic was not cleaned, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "events", ".pooly-doctor.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("jsonl diagnostic was not cleaned, err=%v", err)
	}
}

func openTestStore(t *testing.T, dbPath string) *Store {
	t.Helper()
	store, err := Open(context.Background(), testSQLiteOptions(dbPath))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return store
}

func testSQLiteOptions(path string) SQLiteOptions {
	return SQLiteOptions{
		Path:             path,
		CreateParentDirs: true,
		BusyTimeout:      time.Second,
		WAL:              true,
		Synchronous:      "NORMAL",
	}
}

func openRawTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "raw.db")
	db, err := sql.Open(driverName, sqliteDSN(path, time.Second))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if err := initializeConnection(context.Background(), db, SQLiteOptions{Path: path, BusyTimeout: time.Second, WAL: true, Synchronous: "NORMAL"}); err != nil {
		t.Fatalf("initializeConnection() error = %v", err)
	}
	return db
}

func assertRestrictiveFileMode(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	mode := info.Mode().Perm()
	if mode&0o007 != 0 {
		t.Fatalf("file mode %o allows other access", mode)
	}
	if mode&0o020 != 0 {
		t.Fatalf("file mode %o allows group write", mode)
	}
	if mode&0o200 == 0 || mode&0o400 == 0 {
		t.Fatalf("file mode %o must allow owner read/write", mode)
	}
}
