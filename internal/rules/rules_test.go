package rules

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type mutableClock struct {
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	return c.now.UTC()
}

type memoryRuleStore struct {
	mu        sync.Mutex
	states    map[string]storage.RuleEvaluationStateRecord
	incidents map[string]storage.IncidentRecord
	failGet   bool
	failWrite bool
}

func newMemoryRuleStore() *memoryRuleStore {
	return &memoryRuleStore{
		states:    map[string]storage.RuleEvaluationStateRecord{},
		incidents: map[string]storage.IncidentRecord{},
	}
}

func (s *memoryRuleStore) GetRuleEvaluationState(ctx context.Context, ruleID string, target string) (storage.RuleEvaluationStateRecord, error) {
	if err := ctx.Err(); err != nil {
		return storage.RuleEvaluationStateRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failGet {
		return storage.RuleEvaluationStateRecord{}, errors.New("state get failed")
	}
	record, ok := s.states[ruleID+"\x00"+target]
	if !ok {
		return storage.RuleEvaluationStateRecord{}, storage.ErrNotFound
	}
	return record, nil
}

func (s *memoryRuleStore) UpsertRuleEvaluationState(ctx context.Context, record storage.RuleEvaluationStateRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failWrite {
		return errors.New("state write failed")
	}
	s.states[record.RuleID+"\x00"+record.Target] = record
	return nil
}

func (s *memoryRuleStore) GetIncidentByFingerprint(ctx context.Context, fingerprint string) (storage.IncidentRecord, error) {
	if err := ctx.Err(); err != nil {
		return storage.IncidentRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failGet {
		return storage.IncidentRecord{}, errors.New("incident get failed")
	}
	record, ok := s.incidents[fingerprint]
	if !ok {
		return storage.IncidentRecord{}, storage.ErrNotFound
	}
	return record, nil
}

func (s *memoryRuleStore) UpsertIncident(ctx context.Context, record storage.IncidentRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failWrite {
		return errors.New("incident write failed")
	}
	s.incidents[record.Fingerprint] = record
	return nil
}

func TestThresholdComparisons(t *testing.T) {
	metric := resources.Metric{Name: "pooly_test_ratio", Value: 0.75, Labels: map[string]string{"collector": "test"}}
	event := resources.Event{Category: "kernel_oom"}
	cases := []struct {
		name      string
		threshold Threshold
		match     matchResult
		want      bool
	}{
		{"greater", Threshold{Operator: OpGreaterThan, Value: Value{Number: 0.7, Kind: "number"}}, matchResult{Metric: &metric}, true},
		{"greater equal", Threshold{Operator: OpGreaterThanOrEqual, Value: Value{Number: 0.75, Kind: "number"}}, matchResult{Metric: &metric}, true},
		{"less", Threshold{Operator: OpLessThan, Value: Value{Number: 0.8, Kind: "number"}}, matchResult{Metric: &metric}, true},
		{"less equal", Threshold{Operator: OpLessThanOrEqual, Value: Value{Number: 0.75, Kind: "number"}}, matchResult{Metric: &metric}, true},
		{"equal numeric", Threshold{Operator: OpEqual, Value: Value{Number: 0.75, Kind: "number"}}, matchResult{Metric: &metric}, true},
		{"not equal numeric", Threshold{Operator: OpNotEqual, Value: Value{Number: 0.5, Kind: "number"}}, matchResult{Metric: &metric}, true},
		{"boolean true", Threshold{Operator: OpBooleanTrue}, matchResult{Metric: &metric}, true},
		{"boolean false", Threshold{Operator: OpBooleanFalse}, matchResult{Metric: &metric}, false},
		{"event category", Threshold{Operator: OpEventCategoryMatch, Value: Value{String: "kernel_oom", Kind: "string"}}, matchResult{Event: &event}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := thresholdMatches(tc.threshold, tc.match); got != tc.want {
				t.Fatalf("thresholdMatches() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestSustainedPendingResetEscalationAndRecovery(t *testing.T) {
	store := newMemoryRuleStore()
	clock := &mutableClock{now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}
	rule := Rule{
		ID:          "memory-low",
		Enabled:     true,
		Collector:   "resources",
		Metric:      "pooly_memory_available_ratio",
		Target:      "system",
		Warn:        &Threshold{Operator: OpLessThan, Value: Value{Number: 0.20, Kind: "number"}, For: 10 * time.Second},
		Fail:        &Threshold{Operator: OpLessThan, Value: Value{Number: 0.10, Kind: "number"}, For: 10 * time.Second},
		Critical:    &Threshold{Operator: OpLessThan, Value: Value{Number: 0.05, Kind: "number"}, For: 5 * time.Second},
		RecoverFor:  5 * time.Second,
		MissingData: PolicyStale,
		StaleData:   PolicyStale,
	}
	engine := Engine{Rules: []Rule{rule}, NodeID: "node-001", Clock: clock}

	got := evalOne(t, engine, store, memoryObservation(0.15, clock.now))
	if got.State != StatePendingWarn || len(store.incidents) != 0 {
		t.Fatalf("first warning result = %+v incidents=%d", got, len(store.incidents))
	}
	clock.now = clock.now.Add(5 * time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.50, clock.now))
	if got.State != StateOK || len(store.incidents) != 0 {
		t.Fatalf("pending reset result = %+v incidents=%d", got, len(store.incidents))
	}
	clock.now = clock.now.Add(time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.15, clock.now))
	if got.State != StatePendingWarn {
		t.Fatalf("second pending result = %+v", got)
	}
	clock.now = clock.now.Add(11 * time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.15, clock.now))
	if got.State != StateWarn || got.IncidentTransition == nil || got.IncidentTransition.Action != incidents.ActionOpened {
		t.Fatalf("warn result = %+v", got)
	}
	clock.now = clock.now.Add(time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.08, clock.now))
	if got.State != StatePendingFail || got.Severity != SeverityWarning {
		t.Fatalf("pending fail result = %+v", got)
	}
	clock.now = clock.now.Add(11 * time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.08, clock.now))
	if got.State != StateFail || got.IncidentTransition == nil || got.IncidentTransition.Action != incidents.ActionEscalated {
		t.Fatalf("fail result = %+v", got)
	}
	clock.now = clock.now.Add(time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.03, clock.now))
	if got.State != StatePendingFail {
		t.Fatalf("critical pending result = %+v", got)
	}
	clock.now = clock.now.Add(6 * time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.03, clock.now))
	if got.State != StateCritical || got.Severity != SeverityCritical {
		t.Fatalf("critical result = %+v", got)
	}
	clock.now = clock.now.Add(time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.50, clock.now))
	if got.State != StateRecovering {
		t.Fatalf("recovering result = %+v", got)
	}
	clock.now = clock.now.Add(6 * time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.50, clock.now))
	if got.State != StateRecovered || got.IncidentTransition == nil || got.IncidentTransition.Action != incidents.ActionResolved {
		t.Fatalf("recovered result = %+v", got)
	}
	clock.now = clock.now.Add(time.Second)
	got = evalOne(t, engine, store, memoryObservation(0.50, clock.now))
	if got.State != StateOK {
		t.Fatalf("ok after recovered result = %+v", got)
	}
}

func TestMissingUnsupportedStaleAndCounterResetPolicies(t *testing.T) {
	clock := &mutableClock{now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}
	rule := Rule{
		ID:          "network-required",
		Enabled:     true,
		Collector:   "network",
		Metric:      "pooly_network_interface_up",
		Target:      "eth0",
		Fail:        &Threshold{Operator: OpBooleanFalse, For: time.Second},
		RecoverFor:  time.Second,
		MissingData: PolicyFail,
		StaleData:   PolicyWarn,
	}
	engine := Engine{Rules: []Rule{rule}, NodeID: "node-001", Clock: clock}
	store := newMemoryRuleStore()
	got := evalOne(t, engine, store, nil)
	if got.State != StatePendingFail {
		t.Fatalf("missing result = %+v", got)
	}
	clock.now = clock.now.Add(2 * time.Second)
	got = evalOne(t, engine, store, nil)
	if got.State != StateFail {
		t.Fatalf("missing fail result = %+v", got)
	}
	store = newMemoryRuleStore()
	got = evalOne(t, engine, store, []resources.Observation{{Collector: "network", Target: "all", Supported: false, ErrorClass: resources.ErrorUnsupported}})
	if got.State != StateUnknown || len(store.incidents) != 0 {
		t.Fatalf("unsupported result = %+v incidents=%d", got, len(store.incidents))
	}
	got = evalOne(t, engine, store, []resources.Observation{{Collector: "network", Target: "eth0", Success: true, Supported: true, Stale: true, ErrorClass: resources.ErrorCounterReset}})
	if got.State != StateStale || len(store.incidents) != 0 {
		t.Fatalf("counter reset result = %+v incidents=%d", got, len(store.incidents))
	}
}

func TestPersistedPendingStateSurvivesRestart(t *testing.T) {
	store := newMemoryRuleStore()
	clock := &mutableClock{now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}
	rule := Rule{
		ID:         "cpu-high",
		Enabled:    true,
		Collector:  "cpu",
		Metric:     "pooly_cpu_used_ratio",
		Target:     "system",
		Warn:       &Threshold{Operator: OpGreaterThan, Value: Value{Number: 0.8, Kind: "number"}, For: 10 * time.Second},
		RecoverFor: time.Second,
	}
	engine := Engine{Rules: []Rule{rule}, NodeID: "node-001", Clock: clock}
	got := evalOne(t, engine, store, cpuObservation(0.9, clock.now))
	if got.State != StatePendingWarn {
		t.Fatalf("pending result = %+v", got)
	}
	clock.now = clock.now.Add(11 * time.Second)
	restarted := Engine{Rules: []Rule{rule}, NodeID: "node-001", Clock: clock}
	got = evalOne(t, restarted, store, cpuObservation(0.9, clock.now))
	if got.State != StateWarn || got.IncidentTransition == nil {
		t.Fatalf("restarted result = %+v", got)
	}
}

func TestEvaluationCancellationAndRepositoryFailures(t *testing.T) {
	rule := Rule{
		ID:        "cpu-high",
		Enabled:   true,
		Collector: "cpu",
		Metric:    "pooly_cpu_used_ratio",
		Target:    "system",
		Warn:      &Threshold{Operator: OpGreaterThan, Value: Value{Number: 0.8, Kind: "number"}},
	}
	engine := Engine{Rules: []Rule{rule}, NodeID: "node-001", Clock: StaticClock{At: time.Now().UTC()}}
	store := newMemoryRuleStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.Evaluate(ctx, store, cpuObservation(0.9, time.Now().UTC())); err == nil {
		t.Fatal("Evaluate() canceled error = nil")
	}
	store.failWrite = true
	if _, err := engine.Evaluate(context.Background(), store, cpuObservation(0.9, time.Now().UTC())); err == nil {
		t.Fatal("Evaluate() write failure error = nil")
	}
}

func TestConcurrentEvaluationRaceSafe(t *testing.T) {
	rule := Rule{
		ID:        "cpu-high",
		Enabled:   true,
		Collector: "cpu",
		Metric:    "pooly_cpu_used_ratio",
		Target:    "system",
		Warn:      &Threshold{Operator: OpGreaterThan, Value: Value{Number: 0.8, Kind: "number"}, For: time.Second},
	}
	store := newMemoryRuleStore()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			clock := StaticClock{At: time.Date(2026, 7, 4, 12, 0, i, 0, time.UTC)}
			engine := Engine{Rules: []Rule{rule}, NodeID: "node-001", Clock: clock}
			_, err := engine.Evaluate(context.Background(), store, cpuObservation(0.9, clock.At))
			if err != nil {
				t.Errorf("Evaluate() error = %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func evalOne(t *testing.T, engine Engine, store *memoryRuleStore, observations []resources.Observation) Result {
	t.Helper()
	evaluation, err := engine.Evaluate(context.Background(), store, observations)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(evaluation.Results) != 1 {
		t.Fatalf("results = %d, want 1: %+v", len(evaluation.Results), evaluation.Results)
	}
	return evaluation.Results[0]
}

func memoryObservation(value float64, ts time.Time) []resources.Observation {
	return []resources.Observation{{
		Collector: "memory",
		Target:    "system",
		Timestamp: ts,
		Success:   true,
		Supported: true,
		Metrics: []resources.Metric{{
			Name:      "pooly_memory_available_ratio",
			Value:     value,
			Kind:      resources.MetricGauge,
			Unit:      "ratio",
			Timestamp: ts,
		}},
	}}
}

func cpuObservation(value float64, ts time.Time) []resources.Observation {
	return []resources.Observation{{
		Collector: "cpu",
		Target:    "all",
		Timestamp: ts,
		Success:   true,
		Supported: true,
		Metrics: []resources.Metric{{
			Name:      "pooly_cpu_used_ratio",
			Value:     value,
			Kind:      resources.MetricGauge,
			Unit:      "ratio",
			Labels:    map[string]string{"cpu": "all"},
			Timestamp: ts,
		}},
	}}
}
