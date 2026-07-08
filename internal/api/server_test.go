package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/agent"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/reports"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

func TestServerDisabledStartDoesNothing(t *testing.T) {
	server, err := NewServer(Options{Enabled: false})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if server.Addr() != "" {
		t.Fatalf("Addr() = %q, want empty", server.Addr())
	}
}

func TestServerRejectsNonLoopbackByDefault(t *testing.T) {
	if _, err := NewServer(Options{Enabled: true, Listen: "0.0.0.0:9587"}); err == nil {
		t.Fatal("NewServer() error = nil, want non-loopback rejection")
	}
	if _, err := NewServer(Options{Enabled: true, Listen: "0.0.0.0:9587", AllowNonLoopback: true}); err != nil {
		t.Fatalf("NewServer() explicit allow error = %v", err)
	}
}

func TestHealthAndReadyEndpoints(t *testing.T) {
	server := testServer(t)
	rec := perform(server, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d", rec.Code)
	}
	assertJSONHeaders(t, rec)
	rec = perform(server, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready before SetReady status = %d", rec.Code)
	}
	server.SetReady(true)
	rec = perform(server, "/readyz")
	if rec.Code != http.StatusOK {
		t.Fatalf("ready after SetReady status = %d", rec.Code)
	}
}

func TestStatusEndpointRedactsStorageErrors(t *testing.T) {
	server, err := NewServer(Options{
		Enabled: true,
		Listen:  "127.0.0.1:0",
		Store:   failingStore{err: "token=supersecret"},
		Now:     fixedNow,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	rec := perform(server, "/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "supersecret") || !strings.Contains(body, "[REDACTED]") {
		t.Fatalf("status body was not redacted: %s", body)
	}
}

func TestStatusEndpointIncludesSchedulerStatus(t *testing.T) {
	server, err := NewServer(Options{
		Enabled: true,
		Listen:  "127.0.0.1:0",
		Store:   testStore(t),
		Now:     fixedNow,
		SchedulerStatus: func() agent.SchedulerStatus {
			return agent.SchedulerStatus{Enabled: true, Running: true, Interval: "1m0s", LastSafeErrorSummary: "token=supersecret"}
		},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	rec := perform(server, "/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d", rec.Code)
	}
	var response StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !response.Scheduler.Enabled || response.Scheduler.Interval != "1m0s" {
		t.Fatalf("scheduler status = %+v", response.Scheduler)
	}
	if strings.Contains(rec.Body.String(), "supersecret") {
		t.Fatalf("scheduler status leaked secret: %s", rec.Body.String())
	}
}

func TestIncidentAndDeliveryEndpoints(t *testing.T) {
	store := testStore(t)
	incident := seedIncident(t, store, "inc-1", "open", "warning")
	seedDelivery(t, store, incident.ID, "del-1", "delivered")
	server := testServerWithStore(t, store)

	rec := perform(server, "/incidents")
	if rec.Code != http.StatusOK {
		t.Fatalf("incidents status = %d body=%s", rec.Code, rec.Body.String())
	}
	var incidents []IncidentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &incidents); err != nil {
		t.Fatalf("decode incidents: %v", err)
	}
	if len(incidents) != 1 || strings.Contains(incidents[0].Summary, "secret") {
		t.Fatalf("incidents = %+v", incidents)
	}

	rec = perform(server, "/incidents/"+incident.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("incident detail status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = perform(server, "/notifications/deliveries?incident_id="+incident.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("deliveries status = %d body=%s", rec.Code, rec.Body.String())
	}
	var deliveries []DeliveryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &deliveries); err != nil {
		t.Fatalf("decode deliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Receiver != "local-webhook" {
		t.Fatalf("deliveries = %+v", deliveries)
	}
}

func TestAPIEvidencePathSafety(t *testing.T) {
	store := testStore(t)
	root := filepath.Join(t.TempDir(), "logs")
	inside := filepath.Join(root, "incidents", "open", "id", "evidence.v1.json")
	server, err := NewServer(Options{Enabled: true, Listen: "127.0.0.1:0", Store: store, EvidenceRoot: root})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	incident := seedIncidentWithEvidence(t, store, "inc-evidence-safe", inside)
	rec := perform(server, "/incidents/"+incident.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("incident status = %d body=%s", rec.Code, rec.Body.String())
	}
	var response IncidentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode incident: %v", err)
	}
	if response.EvidencePath != filepath.Join("incidents", "open", "id", "evidence.v1.json") {
		t.Fatalf("evidence path = %q", response.EvidencePath)
	}
	for i, unsafe := range []string{"https://example.test/evidence.json", "incidents/open/../secret.json", "incidents/open/id/token=supersecret.json", filepath.Join(t.TempDir(), "outside", "evidence.json")} {
		incident = seedIncidentWithEvidence(t, store, fmt.Sprintf("inc-evidence-unsafe-%d", i), unsafe)
		rec = perform(server, "/incidents/"+incident.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("unsafe incident status = %d body=%s", rec.Code, rec.Body.String())
		}
		response = IncidentResponse{}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode unsafe incident: %v", err)
		}
		if response.EvidencePath != "" {
			t.Fatalf("unsafe evidence path %q rendered as %q", unsafe, response.EvidencePath)
		}
	}
}

func TestReportEndpoint(t *testing.T) {
	store := testStore(t)
	seedIncident(t, store, "inc-1", "open", "critical")
	seedIncident(t, store, "inc-2", "resolved", "none")
	seedDelivery(t, store, "inc-1", "del-1", "failed")
	server := testServerWithStore(t, store)

	rec := perform(server, "/reports/summary")
	if rec.Code != http.StatusOK {
		t.Fatalf("report status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "supersecret") {
		t.Fatalf("report leaked secret: %s", body)
	}
	if !strings.Contains(body, "production scheduler is disabled unless explicitly configured") {
		t.Fatalf("report missing limitation: %s", body)
	}
}

func TestMalformedPathsAndLimit(t *testing.T) {
	server := testServer(t)
	if rec := perform(server, "/incidents/a/b"); rec.Code != http.StatusNotFound {
		t.Fatalf("malformed incident status = %d", rec.Code)
	}
	if rec := perform(server, "/incidents?limit=999999"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad limit status = %d", rec.Code)
	}
	if rec := performMethod(server, http.MethodPost, "/status"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post status = %d", rec.Code)
	}
}

func TestServeErrorReflectedInStatus(t *testing.T) {
	listener := &errorListener{err: errors.New("serve failed token=supersecret")}
	server, err := NewServer(Options{Enabled: true, Listen: "127.0.0.1:0", Listener: listener, Store: testStore(t)})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	server.SetReady(true)
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && server.Ready() {
		time.Sleep(10 * time.Millisecond)
	}
	if server.Ready() {
		t.Fatal("server stayed ready after Serve error")
	}
	rec := perform(server, "/status")
	var response StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if response.ErrorClass != "api_serve" || response.Readiness {
		t.Fatalf("status = %+v", response)
	}
	if strings.Contains(rec.Body.String(), "supersecret") || !strings.Contains(rec.Body.String(), "[REDACTED]") {
		t.Fatalf("serve error was not redacted: %s", rec.Body.String())
	}
}

func TestWriteJSONHeadersAndEncodingError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]any{"bad": func() {}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	assertJSONHeaders(t, rec)
	if !strings.Contains(rec.Body.String(), "json response could not be encoded") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGracefulShutdownWithInjectedListener(t *testing.T) {
	listener := newFakeListener()
	server, err := NewServer(Options{Enabled: true, Listen: "127.0.0.1:0", Listener: listener, Store: testStore(t)})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if server.Addr() == "" {
		t.Fatal("Addr() empty after start")
	}
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestStartHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server, err := NewServer(Options{Enabled: true, Listen: "127.0.0.1:0", Listener: newFakeListener(), Store: testStore(t)})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if err := server.Start(ctx); err == nil {
		t.Fatal("Start() error = nil, want cancellation")
	}
}

func assertJSONHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	return testServerWithStore(t, testStore(t))
}

func testServerWithStore(t *testing.T, store *storage.Store) *Server {
	t.Helper()
	server, err := NewServer(Options{
		Enabled: true,
		Listen:  "127.0.0.1:0",
		Store:   store,
		Reports: reports.Options{Enabled: true, MaxIncidents: 100, IncludeResolved: true},
		Now:     fixedNow,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return server
}

func perform(server *Server, target string) *httptest.ResponseRecorder {
	return performMethod(server, http.MethodGet, target)
}

func performMethod(server *Server, method string, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	return rec
}

func testStore(t *testing.T) *storage.Store {
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

func seedIncidentWithEvidence(t *testing.T, store *storage.Store, id string, evidencePath string) storage.IncidentRecord {
	t.Helper()
	now := fixedNow()
	record := storage.IncidentRecord{
		ID:              id,
		Fingerprint:     "node:rule:system:" + id,
		NodeID:          "node-1",
		Type:            "rule",
		Target:          "system",
		Condition:       "condition-" + id,
		Severity:        "warning",
		Status:          "open",
		Summary:         "summary",
		FirstSeen:       now.Add(-time.Hour),
		LastSeen:        now,
		OccurrenceCount: 1,
		EvidencePath:    evidencePath,
		LastTransition:  &now,
		UpdatedAt:       now,
	}
	if err := store.UpsertIncident(context.Background(), record); err != nil {
		t.Fatalf("seed incident evidence: %v", err)
	}
	return record
}

func seedIncident(t *testing.T, store *storage.Store, id string, status string, severity string) storage.IncidentRecord {
	t.Helper()
	now := fixedNow()
	record := storage.IncidentRecord{
		ID:              id,
		Fingerprint:     "node:rule:system:" + id,
		NodeID:          "node-1",
		Type:            "rule",
		Target:          "system",
		Condition:       "condition-" + id,
		Severity:        severity,
		Status:          status,
		Summary:         "summary token=supersecret",
		FirstSeen:       now.Add(-time.Hour),
		LastSeen:        now,
		OccurrenceCount: 1,
		LastTransition:  &now,
		UpdatedAt:       now,
	}
	if status == "resolved" {
		record.ResolvedAt = &now
	}
	if err := store.UpsertIncident(context.Background(), record); err != nil {
		t.Fatalf("seed incident: %v", err)
	}
	return record
}

func seedDelivery(t *testing.T, store *storage.Store, incidentID string, id string, status string) {
	t.Helper()
	now := fixedNow()
	if err := store.InsertNotificationDelivery(context.Background(), storage.NotificationDeliveryRecord{
		ID:           id,
		IncidentID:   incidentID,
		Receiver:     "local-webhook",
		CostClass:    "free_core",
		Status:       status,
		Attempt:      1,
		AttemptedAt:  now,
		ErrorClass:   "http",
		ErrorSummary: "token=supersecret",
	}); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
}

type failingStore struct {
	err string
}

func (s failingStore) Ping(context.Context) error { return fmt.Errorf("%s", s.err) }
func (s failingStore) SchemaVersion(context.Context) (int, error) {
	return 0, storage.ErrClosed
}
func (s failingStore) IncidentStatusCounts(context.Context) (map[string]int64, error) {
	return nil, storage.ErrClosed
}
func (s failingStore) NotificationDeliveryStatusCounts(context.Context) (map[string]int64, error) {
	return nil, storage.ErrClosed
}
func (s failingStore) ListRecentIncidents(context.Context, int) ([]storage.IncidentRecord, error) {
	return nil, storage.ErrClosed
}
func (s failingStore) GetIncident(context.Context, string) (storage.IncidentRecord, error) {
	return storage.IncidentRecord{}, storage.ErrClosed
}
func (s failingStore) ListRecentNotificationDeliveries(context.Context, string, int) ([]storage.NotificationDeliveryRecord, error) {
	return nil, storage.ErrClosed
}

type errorListener struct {
	err error
}

func (l *errorListener) Accept() (net.Conn, error) {
	return nil, l.err
}

func (l *errorListener) Close() error   { return nil }
func (l *errorListener) Addr() net.Addr { return fakeAddr("127.0.0.1:0") }

type fakeListener struct {
	closed chan struct{}
}

func newFakeListener() *fakeListener {
	return &fakeListener{closed: make(chan struct{})}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, net.ErrClosed
}

func (l *fakeListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

func (l *fakeListener) Addr() net.Addr {
	return fakeAddr("127.0.0.1:0")
}

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }
