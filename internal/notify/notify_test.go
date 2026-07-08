package notify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/config"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type staticClock struct {
	at time.Time
}

func (c staticClock) Now() time.Time {
	return c.at
}

type memoryStore struct {
	mu         sync.Mutex
	incidents  map[string]storage.IncidentRecord
	deliveries map[string][]storage.NotificationDeliveryRecord
	failGet    bool
	failList   bool
	failTx     bool
}

func newMemoryStore(incident storage.IncidentRecord) *memoryStore {
	return &memoryStore{
		incidents:  map[string]storage.IncidentRecord{incident.ID: incident},
		deliveries: map[string][]storage.NotificationDeliveryRecord{},
	}
}

func (s *memoryStore) GetIncident(ctx context.Context, id string) (storage.IncidentRecord, error) {
	if err := ctx.Err(); err != nil {
		return storage.IncidentRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failGet {
		return storage.IncidentRecord{}, errors.New("get failed")
	}
	incident, ok := s.incidents[id]
	if !ok {
		return storage.IncidentRecord{}, storage.ErrNotFound
	}
	return incident, nil
}

func (s *memoryStore) ListNotificationDeliveries(ctx context.Context, incidentID string) ([]storage.NotificationDeliveryRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failList {
		return nil, errors.New("list failed")
	}
	return append([]storage.NotificationDeliveryRecord(nil), s.deliveries[incidentID]...), nil
}

func (s *memoryStore) NotificationDeliveryTransaction(ctx context.Context, fn func(storage.NotificationDeliveryTransaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failTx {
		return errors.New("transaction failed")
	}
	tx := &memoryTx{}
	if err := fn(tx); err != nil {
		return err
	}
	for _, delivery := range tx.deliveries {
		s.deliveries[delivery.IncidentID] = append(s.deliveries[delivery.IncidentID], delivery)
	}
	for incidentID, alertedAt := range tx.alerted {
		incident := s.incidents[incidentID]
		incident.LastAlerted = &alertedAt
		s.incidents[incidentID] = incident
	}
	return nil
}

type memoryTx struct {
	deliveries []storage.NotificationDeliveryRecord
	alerted    map[string]time.Time
}

func (tx *memoryTx) InsertNotificationDelivery(ctx context.Context, record storage.NotificationDeliveryRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx.deliveries = append(tx.deliveries, record)
	return nil
}

func (tx *memoryTx) UpdateIncidentLastAlerted(ctx context.Context, incidentID string, alertedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx.alerted == nil {
		tx.alerted = map[string]time.Time{}
	}
	tx.alerted[incidentID] = alertedAt
	return nil
}

type fakeReceiver struct {
	mu         sync.Mutex
	id         string
	enabled    bool
	events     []Event
	severities []incidents.Severity
	success    bool
	class      string
	summary    string
	calls      int
}

func (r *fakeReceiver) ID() string        { return r.id }
func (r *fakeReceiver) Enabled() bool     { return r.enabled }
func (r *fakeReceiver) Type() string      { return "fake" }
func (r *fakeReceiver) CostClass() string { return "free_core" }
func (r *fakeReceiver) Summary() string   { return r.id }
func (r *fakeReceiver) Matches(e Event, s incidents.Severity) bool {
	if len(r.events) > 0 && !containsEvent(r.events, e) {
		return false
	}
	if e == EventResolved {
		return true
	}
	return len(r.severities) == 0 || containsSeverity(r.severities, s)
}
func (r *fakeReceiver) Deliver(ctx context.Context, payload Payload) DeliveryOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if err := ctx.Err(); err != nil {
		return DeliveryOutcome{Success: false, ErrorClass: "canceled", Summary: "canceled"}
	}
	return DeliveryOutcome{Success: r.success, ErrorClass: r.class, Summary: r.summary}
}

func containsEvent(values []Event, target Event) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsSeverity(values []incidents.Severity, target incidents.Severity) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestDisabledNotificationAndReceiverFiltering(t *testing.T) {
	incident := testIncident("warning")
	store := newMemoryStore(incident)
	service := Service{Enabled: false, Store: store, Receivers: []Receiver{&fakeReceiver{id: "r1", enabled: true, success: true}}, Clock: staticClock{at: testNow()}}
	report, err := service.DeliverIncident(context.Background(), incident, EventOpened)
	if err != nil {
		t.Fatalf("DeliverIncident() error = %v", err)
	}
	if report.Skipped != 1 {
		t.Fatalf("disabled report = %+v", report)
	}

	receiver := &fakeReceiver{id: "r2", enabled: false, success: true}
	service = Service{Enabled: true, Store: store, Receivers: []Receiver{receiver}, Clock: staticClock{at: testNow()}}
	report, err = service.DeliverIncident(context.Background(), incident, EventOpened)
	if err != nil {
		t.Fatalf("DeliverIncident() disabled receiver error = %v", err)
	}
	if report.Skipped != 1 || receiver.calls != 0 {
		t.Fatalf("disabled receiver report = %+v calls=%d", report, receiver.calls)
	}
}

func TestSeverityAndEventFiltering(t *testing.T) {
	incident := testIncident("warning")
	store := newMemoryStore(incident)
	receiver := &fakeReceiver{
		id:         "r1",
		enabled:    true,
		success:    true,
		events:     []Event{EventEscalated},
		severities: []incidents.Severity{incidents.SeverityCritical},
	}
	service := Service{Enabled: true, Store: store, Receivers: []Receiver{receiver}, Clock: staticClock{at: testNow()}}
	report, err := service.DeliverIncident(context.Background(), incident, EventOpened)
	if err != nil {
		t.Fatalf("DeliverIncident() error = %v", err)
	}
	if report.Skipped != 1 || receiver.calls != 0 {
		t.Fatalf("event filtered report = %+v calls=%d", report, receiver.calls)
	}
	incident.Severity = string(incidents.SeverityCritical)
	report, err = service.DeliverIncident(context.Background(), incident, EventEscalated)
	if err != nil {
		t.Fatalf("DeliverIncident() escalated error = %v", err)
	}
	if report.Delivered != 1 || receiver.calls != 1 {
		t.Fatalf("escalated report = %+v calls=%d", report, receiver.calls)
	}
}

func TestOpenedEscalatedResolvedDeliveryAndLastAlerted(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event Event
	}{
		{name: "opened", event: EventOpened},
		{name: "escalated", event: EventEscalated},
		{name: "resolved", event: EventResolved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			incident := testIncident("failure")
			if tc.event == EventResolved {
				incident.Status = string(incidents.StatusResolved)
				incident.Severity = string(incidents.SeverityNone)
				resolved := testNow()
				incident.ResolvedAt = &resolved
			}
			store := newMemoryStore(incident)
			receiver := &fakeReceiver{id: "r1", enabled: true, success: true, events: []Event{tc.event}, summary: "ok"}
			service := Service{Enabled: true, Store: store, Receivers: []Receiver{receiver}, Clock: staticClock{at: testNow()}}
			report, err := service.DeliverIncident(context.Background(), incident, tc.event)
			if err != nil {
				t.Fatalf("DeliverIncident() error = %v", err)
			}
			if report.Delivered != 1 {
				t.Fatalf("report = %+v", report)
			}
			got := store.incidents[incident.ID]
			if got.LastAlerted == nil {
				t.Fatal("LastAlerted was not updated")
			}
			if len(store.deliveries[incident.ID]) != 1 || store.deliveries[incident.ID][0].Status != string(StatusDelivered) {
				t.Fatalf("deliveries = %+v", store.deliveries[incident.ID])
			}
		})
	}
}

func TestDuplicateSuppressionAndRetryAfterFailure(t *testing.T) {
	incident := testIncident("failure")
	store := newMemoryStore(incident)
	receiver := &fakeReceiver{id: "r1", enabled: true, success: false, class: "http_status", summary: "failed"}
	service := Service{Enabled: true, Store: store, Receivers: []Receiver{receiver}, Clock: staticClock{at: testNow()}}
	report, err := service.DeliverIncident(context.Background(), incident, EventOpened)
	if err != nil {
		t.Fatalf("failed delivery persistence error = %v", err)
	}
	if report.Failed != 1 || store.incidents[incident.ID].LastAlerted != nil {
		t.Fatalf("failed report = %+v incident=%+v", report, store.incidents[incident.ID])
	}
	receiver.success = true
	receiver.summary = "ok"
	service.Clock = staticClock{at: testNow().Add(time.Minute)}
	report, err = service.DeliverIncident(context.Background(), incident, EventOpened)
	if err != nil {
		t.Fatalf("retry delivery error = %v", err)
	}
	if report.Delivered != 1 || report.Results[0].Attempt != 2 {
		t.Fatalf("retry report = %+v", report)
	}
	report, err = service.DeliverIncident(context.Background(), incident, EventOpened)
	if err != nil {
		t.Fatalf("duplicate delivery error = %v", err)
	}
	if report.Skipped != 1 {
		t.Fatalf("duplicate report = %+v", report)
	}
}

func TestDryRunSendsNothing(t *testing.T) {
	incident := testIncident("warning")
	store := newMemoryStore(incident)
	receiver := &fakeReceiver{id: "r1", enabled: true, success: true}
	service := Service{Enabled: false, DryRun: true, Store: store, Receivers: []Receiver{receiver}, Clock: staticClock{at: testNow()}}
	report, err := service.DeliverIncident(context.Background(), incident, EventOpened)
	if err != nil {
		t.Fatalf("DeliverIncident() error = %v", err)
	}
	if report.DryRun != 1 || receiver.calls != 0 || len(store.deliveries[incident.ID]) != 0 {
		t.Fatalf("dry-run report = %+v calls=%d deliveries=%+v", report, receiver.calls, store.deliveries)
	}
}

func TestTransitionDeliveryLoadsIncident(t *testing.T) {
	incident := testIncident("warning")
	store := newMemoryStore(incident)
	receiver := &fakeReceiver{id: "r1", enabled: true, success: true}
	service := Service{Enabled: true, Store: store, Receivers: []Receiver{receiver}, Clock: staticClock{at: testNow()}}
	report, err := service.DeliverTransitions(context.Background(), []incidents.Transition{{Action: incidents.ActionOpened, IncidentID: incident.ID}})
	if err != nil {
		t.Fatalf("DeliverTransitions() error = %v", err)
	}
	if report.Delivered != 1 {
		t.Fatalf("transition report = %+v", report)
	}
}

func TestContextCancellationAndRepositoryFailures(t *testing.T) {
	incident := testIncident("warning")
	store := newMemoryStore(incident)
	receiver := &fakeReceiver{id: "r1", enabled: true, success: true}
	service := Service{Enabled: true, Store: store, Receivers: []Receiver{receiver}, Clock: staticClock{at: testNow()}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.DeliverIncident(ctx, incident, EventOpened); err == nil {
		t.Fatal("DeliverIncident() canceled error = nil")
	}
	store.failList = true
	if _, err := service.DeliverIncident(context.Background(), incident, EventOpened); err == nil {
		t.Fatal("DeliverIncident() list failure error = nil")
	}
	store.failList = false
	store.failTx = true
	if _, err := service.DeliverIncident(context.Background(), incident, EventOpened); err == nil {
		t.Fatal("DeliverIncident() transaction failure error = nil")
	}
}

func TestWebhookReceiverSuccessFailureTimeoutAndRedaction(t *testing.T) {
	successDoer := handlerDoer{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q", got)
		}
		w.WriteHeader(http.StatusAccepted)
	})}
	success := NewWebhookReceiver(ReceiverSpec{ID: "web", Type: "webhook", Enabled: true, URL: "https://example.test/hook", Timeout: time.Second}, successDoer)
	if got := success.Deliver(context.Background(), RenderPayload(testIncident("warning"), EventOpened)); !got.Success {
		t.Fatalf("success outcome = %+v", got)
	}

	failDoer := handlerDoer{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("token=supersecret " + strings.Repeat("x", maxWebhookResponseBytes*2)))
	})}
	failing := NewWebhookReceiver(ReceiverSpec{ID: "web", Type: "webhook", Enabled: true, URL: "https://example.test/hook", Timeout: time.Second}, failDoer)
	outcome := failing.Deliver(context.Background(), RenderPayload(testIncident("warning"), EventOpened))
	if outcome.Success || outcome.ErrorClass != "http_status" {
		t.Fatalf("failure outcome = %+v", outcome)
	}
	if strings.Contains(outcome.Summary, "supersecret") || len(outcome.Summary) > maxWebhookResponseBytes+200 {
		t.Fatalf("unsafe failure summary = %q", outcome.Summary)
	}

	timeoutDoer := handlerDoer{delay: 50 * time.Millisecond, handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	timeoutReceiver := NewWebhookReceiver(ReceiverSpec{ID: "web", Type: "webhook", Enabled: true, URL: "https://example.test/hook", Timeout: time.Millisecond}, timeoutDoer)
	outcome = timeoutReceiver.Deliver(context.Background(), RenderPayload(testIncident("warning"), EventOpened))
	if outcome.Success || outcome.ErrorClass != "timeout" {
		t.Fatalf("timeout outcome = %+v", outcome)
	}
}

func TestWebhookDefaultClientDoesNotFollowRedirects(t *testing.T) {
	client := noRedirectHTTPClient()
	req := httptest.NewRequest(http.MethodGet, "https://example.test/redirected?token=supersecret", nil)
	if err := client.CheckRedirect(req, []*http.Request{{}}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect() error = %v, want ErrUseLastResponse", err)
	}
}

func TestWebhookReceiverTreatsRedirectStatusAsFailure(t *testing.T) {
	redirectDoer := handlerDoer{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/redirected?token=supersecret")
		w.WriteHeader(http.StatusTemporaryRedirect)
		_, _ = w.Write([]byte(`<a href="/redirected?token=supersecret">redirect</a>`))
	})}
	receiver := NewWebhookReceiver(ReceiverSpec{ID: "web", Type: "webhook", Enabled: true, URL: "https://example.test/hook", Timeout: time.Second}, redirectDoer)
	outcome := receiver.Deliver(context.Background(), RenderPayload(testIncident("warning"), EventOpened))
	if outcome.Success || outcome.ErrorClass != "http_status" || !strings.Contains(outcome.Summary, "HTTP 307") {
		t.Fatalf("redirect outcome = %+v", outcome)
	}
	if strings.Contains(outcome.Summary, "supersecret") || strings.Contains(outcome.Summary, "/redirected") {
		t.Fatalf("redirect summary leaked target: %q", outcome.Summary)
	}
}

func TestRenderPayloadEvidencePathSafety(t *testing.T) {
	root := t.TempDir()
	incident := testIncident("warning")
	incident.EvidencePath = root + "/incidents/open/id/evidence.v1.json"
	payload := RenderPayloadWithEvidenceRoot(incident, EventOpened, root)
	if payload.EvidencePath != "incidents/open/id/evidence.v1.json" {
		t.Fatalf("evidence path = %q", payload.EvidencePath)
	}
	for _, unsafe := range []string{
		"https://example.test/evidence.json",
		"incidents/open/../secret.json",
		"incidents/open/id/token=supersecret.json",
		root + "-other/evidence.json",
	} {
		incident.EvidencePath = unsafe
		payload = RenderPayloadWithEvidenceRoot(incident, EventOpened, root)
		if payload.EvidencePath != "" {
			t.Fatalf("unsafe evidence path %q rendered as %q", unsafe, payload.EvidencePath)
		}
	}
}

func TestOptionsFromConfigWebhookURLValidation(t *testing.T) {
	cfg := config.Default()
	cfg.Notify.Enabled = true
	cfg.Notify.DryRun = false
	cfg.Notify.Receivers = []config.NotifyReceiverConfig{{
		ID:         "web",
		Enabled:    true,
		Type:       "webhook",
		URLEnv:     "POOLY_WEBHOOK_URL",
		Timeout:    config.Duration{Duration: time.Second},
		Events:     []string{"opened"},
		Severities: []string{"warning"},
	}}
	_, err := OptionsFromConfig(cfg, func(string) (string, bool) { return "http://example.test/hook", true })
	if err == nil {
		t.Fatal("OptionsFromConfig() accepted insecure non-local URL")
	}
	cfg.Notify.Receivers[0].AllowInsecureLocal = true
	_, err = OptionsFromConfig(cfg, func(string) (string, bool) { return "http://127.0.0.1/hook?token=secret", true })
	if err != nil {
		t.Fatalf("OptionsFromConfig() local URL error = %v", err)
	}
}

func TestConcurrentDeliveryRaceSafe(t *testing.T) {
	incident := testIncident("warning")
	store := newMemoryStore(incident)
	service := Service{
		Enabled: true,
		Store:   store,
		Receivers: []Receiver{
			&fakeReceiver{id: "r1", enabled: true, success: true},
		},
		Clock: staticClock{at: testNow()},
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = service.DeliverIncident(context.Background(), incident, EventOpened)
		}()
	}
	wg.Wait()
}

func testIncident(severity string) storage.IncidentRecord {
	now := testNow()
	return storage.IncidentRecord{
		ID:              "inc-test",
		Fingerprint:     "node:rule:system:condition",
		NodeID:          "node",
		Type:            "rule",
		Target:          "system",
		Condition:       "condition",
		Severity:        severity,
		Status:          string(incidents.StatusOpen),
		Summary:         "safe summary",
		FirstSeen:       now.Add(-time.Minute),
		LastSeen:        now,
		OccurrenceCount: 1,
		LastTransition:  &now,
	}
}

func testNow() time.Time {
	return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
}

type handlerDoer struct {
	handler http.Handler
	delay   time.Duration
}

func (d handlerDoer) Do(req *http.Request) (*http.Response, error) {
	if d.delay > 0 {
		timer := time.NewTimer(d.delay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	recorder := httptest.NewRecorder()
	d.handler.ServeHTTP(recorder, req)
	return recorder.Result(), nil
}
