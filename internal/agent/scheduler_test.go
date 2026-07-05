package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/notify"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/rules"
)

func TestSchedulerRunOnceSuccess(t *testing.T) {
	store := &memoryStatusStore{}
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			return []resources.Observation{okObservation()}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{}, nil
		}),
		StatusStore: store,
		Now:         fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(result.Observations) != 1 || result.Status.CycleCount != 1 || result.Status.FailedCycleCount != 0 {
		t.Fatalf("result = %+v", result)
	}
	if store.saved.CycleCount != 1 {
		t.Fatalf("persisted status = %+v", store.saved)
	}
}

func TestSchedulerCollectorFailureEvaluatesAndMarksFailure(t *testing.T) {
	evaluated := false
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			return []resources.Observation{{
				Collector:  "resources",
				Target:     "system",
				Timestamp:  fixedSchedulerNow(),
				Success:    false,
				Supported:  true,
				ErrorClass: resources.ErrorParse,
				Summary:    "token=supersecret",
			}}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			evaluated = true
			return rules.Evaluation{}, nil
		}),
		Now: fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce() error = nil, want collector failure")
	}
	if !evaluated {
		t.Fatal("evaluator was not called with collector failure observation")
	}
	if result.Status.LastSafeErrorClass != "collector" || result.Status.FailedCycleCount != 1 {
		t.Fatalf("status = %+v", result.Status)
	}
	if result.Status.LastSafeErrorSummary == "token=supersecret" {
		t.Fatalf("unredacted summary = %q", result.Status.LastSafeErrorSummary)
	}
}

func TestSchedulerUnsupportedAndCounterResetDoNotFailCycle(t *testing.T) {
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			return []resources.Observation{
				{Collector: "systemd", Target: "all", Success: false, Supported: false, ErrorClass: resources.ErrorUnsupported, Summary: "unsupported platform"},
				{Collector: "network", Target: "eth0", Success: false, Supported: true, Stale: true, ErrorClass: resources.ErrorCounterReset, Summary: "counter reset"},
			}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{}, nil
		}),
		Now: fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Status.FailedCycleCount != 0 {
		t.Fatalf("status = %+v", result.Status)
	}
}

func TestSchedulerRuleFailure(t *testing.T) {
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			return []resources.Observation{okObservation()}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{}, errors.New("rule store failed")
		}),
		Now: fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err == nil || result.Status.LastSafeErrorClass != "rule" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestSchedulerIncidentTransitionNotification(t *testing.T) {
	called := false
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			return []resources.Observation{okObservation()}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{Transitions: []incidents.Transition{{Action: incidents.ActionOpened, IncidentID: "inc-1"}}}, nil
		}),
		Notifier: TransitionNotifierFunc(func(context.Context, []incidents.Transition) (notify.Report, error) {
			called = true
			return notify.Report{Delivered: 1}, nil
		}),
		Now: fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !called || result.NotificationReport.Delivered != 1 {
		t.Fatalf("notification called=%t report=%+v", called, result.NotificationReport)
	}
}

func TestSchedulerNotificationFailureMarksCycleFailure(t *testing.T) {
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			return []resources.Observation{okObservation()}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{Transitions: []incidents.Transition{{Action: incidents.ActionOpened, IncidentID: "inc-1"}}}, nil
		}),
		Notifier: TransitionNotifierFunc(func(context.Context, []incidents.Transition) (notify.Report, error) {
			return notify.Report{Failed: 1}, nil
		}),
		Now: fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err == nil || result.Status.LastSafeErrorClass != "notification" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestSchedulerStatusStoreFailureMarksCycleFailure(t *testing.T) {
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			return []resources.Observation{okObservation()}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{}, nil
		}),
		StatusStore: &memoryStatusStore{err: errors.New("storage write failed")},
		Now:         fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err == nil || result.Status.LastSafeErrorClass != "state_error" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Status.LastSuccessfulCycleAt != nil {
		t.Fatalf("LastSuccessfulCycleAt = %v, want nil", result.Status.LastSuccessfulCycleAt)
	}
}

func TestSchedulerTimeoutClassFromStage(t *testing.T) {
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			return nil, context.DeadlineExceeded
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{}, nil
		}),
		Now: fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err == nil || result.Status.LastSafeErrorClass != "timeout" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestSchedulerContextCancellationBeforeEvaluation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	evaluated := false
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			cancel()
			return []resources.Observation{okObservation()}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			evaluated = true
			return rules.Evaluation{}, nil
		}),
		Now: fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(ctx)
	if err == nil || result.Status.LastSafeErrorClass != "canceled" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if evaluated {
		t.Fatal("evaluator was called after context cancellation")
	}
}

func TestSchedulerNoOverlappingCycles(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			close(started)
			<-release
			return []resources.Observation{okObservation()}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{}, nil
		}),
		Now: fixedSchedulerNow,
	})
	done := make(chan error, 1)
	go func() {
		_, err := scheduler.RunOnce(context.Background())
		done <- err
	}()
	<-started
	result, err := scheduler.RunOnce(context.Background())
	if !errors.Is(err, ErrCycleAlreadyRunning) || result.Status.LastSafeErrorClass != "overlap" {
		t.Fatalf("overlap result=%+v err=%v", result, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first RunOnce() error = %v", err)
	}
}

func TestSchedulerContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:   true,
		Interval:  time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) { return nil, ctx.Err() }),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{}, nil
		}),
		Now: fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(ctx)
	if err == nil || result.Status.LastSafeErrorClass != "canceled" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestSchedulerRunOnStartAndStop(t *testing.T) {
	cycles := 0
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:    true,
		Interval:   time.Hour,
		RunOnStart: true,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			cycles++
			return []resources.Observation{okObservation()}, nil
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{}, nil
		}),
		Now: fixedSchedulerNow,
		TickerFactory: func(time.Duration) SchedulerTicker {
			return manualTicker{ch: make(chan time.Time)}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if cycles != 1 || !scheduler.Status().Running {
		t.Fatalf("cycles=%d status=%+v", cycles, scheduler.Status())
	}
	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := scheduler.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestSchedulerPanicRecovery(t *testing.T) {
	scheduler := NewScheduler(SchedulerOptions{
		Enabled:  true,
		Interval: time.Minute,
		Collector: CollectorFunc(func(context.Context) ([]resources.Observation, error) {
			panic("boom")
		}),
		Evaluator: EvaluatorFunc(func(context.Context, []resources.Observation) (rules.Evaluation, error) {
			return rules.Evaluation{}, nil
		}),
		Now: fixedSchedulerNow,
	})
	result, err := scheduler.RunOnce(context.Background())
	if err == nil || result.Status.LastSafeErrorClass != "internal_error" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func okObservation() resources.Observation {
	return resources.Observation{
		Collector: "resources",
		Target:    "system",
		Timestamp: fixedSchedulerNow(),
		Success:   true,
		Supported: true,
		Summary:   "ok",
	}
}

func fixedSchedulerNow() time.Time {
	return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
}

type memoryStatusStore struct {
	saved SchedulerStatus
	err   error
}

func (s *memoryStatusStore) SaveSchedulerStatus(_ context.Context, status SchedulerStatus) error {
	if s.err != nil {
		return s.err
	}
	s.saved = status
	return nil
}

type manualTicker struct {
	ch chan time.Time
}

func (t manualTicker) C() <-chan time.Time {
	return t.ch
}

func (t manualTicker) Stop() {}
