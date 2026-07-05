package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/notify"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/rules"
)

type Collector interface {
	Collect(ctx context.Context) ([]resources.Observation, error)
}

type CollectorFunc func(ctx context.Context) ([]resources.Observation, error)

func (f CollectorFunc) Collect(ctx context.Context) ([]resources.Observation, error) {
	return f(ctx)
}

type Evaluator interface {
	Evaluate(ctx context.Context, observations []resources.Observation) (rules.Evaluation, error)
}

type EvaluatorFunc func(ctx context.Context, observations []resources.Observation) (rules.Evaluation, error)

func (f EvaluatorFunc) Evaluate(ctx context.Context, observations []resources.Observation) (rules.Evaluation, error) {
	return f(ctx, observations)
}

type TransitionNotifier interface {
	DeliverTransitions(ctx context.Context, transitions []incidents.Transition) (notify.Report, error)
}

type TransitionNotifierFunc func(ctx context.Context, transitions []incidents.Transition) (notify.Report, error)

func (f TransitionNotifierFunc) DeliverTransitions(ctx context.Context, transitions []incidents.Transition) (notify.Report, error) {
	return f(ctx, transitions)
}

type SchedulerStatusStore interface {
	SaveSchedulerStatus(ctx context.Context, status SchedulerStatus) error
}

type SchedulerOptions struct {
	Enabled                bool
	Interval               time.Duration
	RunOnStart             bool
	CycleTimeout           time.Duration
	MaxConsecutiveFailures int
	Collector              Collector
	Evaluator              Evaluator
	Notifier               TransitionNotifier
	StatusStore            SchedulerStatusStore
	Logger                 *slog.Logger
	Now                    func() time.Time
	TickerFactory          func(time.Duration) SchedulerTicker
}

type SchedulerTicker interface {
	C() <-chan time.Time
	Stop()
}

type SchedulerStatus struct {
	Enabled                bool       `json:"enabled"`
	Running                bool       `json:"running"`
	Interval               string     `json:"interval"`
	LastAttemptAt          *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessfulCycleAt  *time.Time `json:"last_successful_cycle_at,omitempty"`
	LastCycleDuration      string     `json:"last_cycle_duration,omitempty"`
	LastSafeErrorClass     string     `json:"last_safe_error_class,omitempty"`
	LastSafeErrorSummary   string     `json:"last_safe_error_summary,omitempty"`
	CycleCount             int64      `json:"cycle_count"`
	FailedCycleCount       int64      `json:"failed_cycle_count"`
	ConsecutiveFailures    int64      `json:"consecutive_failures"`
	CurrentlyRunningCycle  bool       `json:"currently_running_cycle"`
	MaxConsecutiveFailures int        `json:"max_consecutive_failures"`
}

type CycleResult struct {
	Observations       []resources.Observation `json:"observations,omitempty"`
	Evaluation         rules.Evaluation        `json:"evaluation"`
	NotificationReport notify.Report           `json:"notification_report"`
	Status             SchedulerStatus         `json:"status"`
}

var (
	ErrCycleAlreadyRunning = errors.New("scheduler cycle already running")
	ErrCollectorFailure    = errors.New("collector failure")
	ErrNotificationFailure = errors.New("notification delivery failure")
)

type Scheduler struct {
	opts    SchedulerOptions
	mu      sync.Mutex
	status  SchedulerStatus
	cycleMu sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewScheduler(opts SchedulerOptions) *Scheduler {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Interval <= 0 {
		opts.Interval = time.Minute
	}
	if opts.CycleTimeout <= 0 {
		opts.CycleTimeout = opts.Interval / 2
	}
	if opts.TickerFactory == nil {
		opts.TickerFactory = func(interval time.Duration) SchedulerTicker {
			return realTicker{ticker: time.NewTicker(interval)}
		}
	}
	status := SchedulerStatus{
		Enabled:                opts.Enabled,
		Interval:               opts.Interval.String(),
		MaxConsecutiveFailures: opts.MaxConsecutiveFailures,
	}
	return &Scheduler{opts: opts, status: status}
}

func (s *Scheduler) Start(ctx context.Context) error {
	if s == nil || !s.opts.Enabled {
		return nil
	}
	if ctx == nil {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.ctx = runCtx
	s.cancel = cancel
	s.done = make(chan struct{})
	s.status.Running = true
	s.mu.Unlock()

	if s.opts.RunOnStart {
		if _, err := s.RunOnce(runCtx); err != nil && s.opts.Logger != nil {
			s.opts.Logger.WarnContext(runCtx, "scheduler run-on-start failed",
				slog.String("component", "scheduler"),
				slog.String("error_class", classifyCycleError(err)),
			)
		}
	}
	go s.loop(runCtx)
	return nil
}

func (s *Scheduler) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.status.Running = false
	s.status.CurrentlyRunningCycle = false
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) RunOnce(ctx context.Context) (result CycleResult, err error) {
	if s == nil {
		return CycleResult{}, fmt.Errorf("scheduler is nil")
	}
	if ctx == nil {
		return CycleResult{}, context.Canceled
	}
	if !s.cycleMu.TryLock() {
		err := safeCycleError{class: "overlap", summary: "scheduler cycle already running", err: ErrCycleAlreadyRunning}
		s.finishCycle(s.now(), 0, err)
		return CycleResult{Status: s.Status()}, err
	}
	defer s.cycleMu.Unlock()

	started := s.now()
	s.markAttempt(started)
	defer func() {
		if recovered := recover(); recovered != nil {
			err = safeCycleError{class: "internal_error", summary: "scheduler cycle panic recovered", err: fmt.Errorf("panic recovered")}
		}
		if finishErr := s.finishCycle(started, s.now().Sub(started), err); finishErr != nil {
			if err == nil {
				err = finishErr
			} else {
				err = errors.Join(err, finishErr)
			}
		}
		result.Status = s.Status()
	}()

	cycleCtx := ctx
	cancel := func() {}
	if s.opts.CycleTimeout > 0 {
		cycleCtx, cancel = context.WithTimeout(ctx, s.opts.CycleTimeout)
	}
	defer cancel()

	if s.opts.Collector == nil {
		err = safeCycleError{class: "collector", summary: "scheduler collector is not configured"}
		return result, err
	}
	observations, collectErr := s.opts.Collector.Collect(cycleCtx)
	result.Observations = observations
	if collectErr != nil {
		err = safeStageError("collector", collectErr)
		return result, err
	}
	if ctxErr := cycleCtx.Err(); ctxErr != nil {
		err = safeStageError("collector", ctxErr)
		return result, err
	}
	collectorErr := classifyCollectorObservations(observations)

	if s.opts.Evaluator == nil {
		err = safeCycleError{class: "rule", summary: "scheduler evaluator is not configured"}
		return result, err
	}
	evaluation, evalErr := s.opts.Evaluator.Evaluate(cycleCtx, observations)
	result.Evaluation = evaluation
	if evalErr != nil {
		err = safeStageError("rule", evalErr)
		return result, err
	}

	if s.opts.Notifier != nil && len(evaluation.Transitions) > 0 {
		report, notifyErr := s.opts.Notifier.DeliverTransitions(cycleCtx, evaluation.Transitions)
		result.NotificationReport = report
		if notifyErr != nil {
			err = safeStageError("notification", notifyErr)
			return result, err
		}
		if notify.Failed(report) {
			err = safeCycleError{class: "notification", summary: "one or more notification deliveries failed", err: ErrNotificationFailure}
			return result, err
		}
	}
	if collectorErr != nil {
		err = collectorErr
		return result, err
	}
	if err := cycleCtx.Err(); err != nil {
		err = safeCycleError{class: "timeout", summary: err.Error(), err: err}
		return result, err
	}
	return result, nil
}

func (s *Scheduler) Status() SchedulerStatus {
	if s == nil {
		return SchedulerStatus{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return SafeSchedulerStatus(s.status)
}

func SafeSchedulerStatus(status SchedulerStatus) SchedulerStatus {
	status.Interval = redaction.Redact(status.Interval)
	status.LastCycleDuration = redaction.Redact(status.LastCycleDuration)
	status.LastSafeErrorClass = redaction.Redact(status.LastSafeErrorClass)
	status.LastSafeErrorSummary = redaction.Redact(status.LastSafeErrorSummary)
	return status
}

func (s *Scheduler) loop(ctx context.Context) {
	defer close(s.done)
	ticker := s.opts.TickerFactory(s.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			if _, err := s.RunOnce(ctx); err != nil && s.opts.Logger != nil {
				s.opts.Logger.WarnContext(ctx, "scheduler cycle failed",
					slog.String("component", "scheduler"),
					slog.String("error_class", classifyCycleError(err)),
				)
			}
		}
	}
}

func (s *Scheduler) markAttempt(started time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := started.UTC()
	s.status.LastAttemptAt = &t
	s.status.CurrentlyRunningCycle = true
	s.status.CycleCount++
}

func (s *Scheduler) finishCycle(started time.Time, duration time.Duration, err error) error {
	s.mu.Lock()
	baseStatus := s.status
	s.mu.Unlock()

	status := s.cycleStatus(baseStatus, started, duration, err)
	if s.opts.StatusStore != nil {
		saveErr := s.opts.StatusStore.SaveSchedulerStatus(context.Background(), status)
		if saveErr != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("scheduler status persistence failed",
				slog.String("component", "scheduler"),
				slog.String("error_class", "state_error"),
			)
		}
		if saveErr != nil {
			persistErr := safeCycleError{class: "state_error", summary: "scheduler status persistence failed", err: saveErr}
			if err == nil {
				status = s.cycleStatus(baseStatus, started, duration, persistErr)
			}
			s.mu.Lock()
			s.status = status
			s.mu.Unlock()
			return persistErr
		}
	}
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) cycleStatus(status SchedulerStatus, started time.Time, duration time.Duration, err error) SchedulerStatus {
	status.CurrentlyRunningCycle = false
	status.LastCycleDuration = duration.String()
	if err != nil {
		status.FailedCycleCount++
		status.ConsecutiveFailures++
		status.LastSafeErrorClass = classifyCycleError(err)
		status.LastSafeErrorSummary = safeSummary(err.Error())
		return status
	}
	t := s.now().UTC()
	if !started.IsZero() {
		t = started.Add(duration).UTC()
	}
	status.LastSuccessfulCycleAt = &t
	status.ConsecutiveFailures = 0
	status.LastSafeErrorClass = ""
	status.LastSafeErrorSummary = ""
	return status
}

func (s *Scheduler) now() time.Time {
	if s.opts.Now != nil {
		return s.opts.Now().UTC()
	}
	return time.Now().UTC()
}

func classifyCollectorObservations(observations []resources.Observation) error {
	for _, observation := range observations {
		if observation.Success || !observation.Supported || observation.ErrorClass == resources.ErrorCounterReset {
			continue
		}
		summary := observation.Summary
		if summary == "" {
			summary = "collector observation failed"
		}
		return safeCycleError{class: "collector", summary: summary, err: ErrCollectorFailure}
	}
	return nil
}

func safeStageError(defaultClass string, err error) safeCycleError {
	class := defaultClass
	if errors.Is(err, context.DeadlineExceeded) {
		class = "timeout"
	} else if errors.Is(err, context.Canceled) {
		class = "canceled"
	}
	return safeCycleError{class: class, summary: err.Error(), err: err}
}

type safeCycleError struct {
	class   string
	summary string
	err     error
}

func (e safeCycleError) Error() string {
	return safeSummary(e.summary)
}

func (e safeCycleError) Unwrap() error {
	return e.err
}

func classifyCycleError(err error) string {
	if err == nil {
		return ""
	}
	var cycleErr safeCycleError
	if errors.As(err, &cycleErr) && cycleErr.class != "" {
		return cycleErr.class
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "internal_error"
}

func safeSummary(value string) string {
	value = redaction.Redact(value)
	if value == "" {
		return "scheduler cycle failed"
	}
	if len(value) > 300 {
		return value[:300]
	}
	return value
}

type realTicker struct {
	ticker *time.Ticker
}

func (t realTicker) C() <-chan time.Time {
	return t.ticker.C
}

func (t realTicker) Stop() {
	t.ticker.Stop()
}
