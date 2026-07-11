package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunInfrastructureReadinessAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := &fakeAPI{readyCh: make(chan struct{})}
	scheduler := &fakeScheduler{startedCh: make(chan struct{})}
	notifier := &fakeNotifier{api: api, readyCh: make(chan struct{})}
	store := &fakeStore{}
	watchdogStarted := make(chan struct{})
	watchdogExited := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- RunInfrastructure(ctx, RuntimeOptions{
			Store:           store,
			API:             api,
			Scheduler:       scheduler,
			Notifier:        notifier,
			ShutdownTimeout: time.Second,
			WatchdogEnabled: true,
			RunWatchdog: func(ctx context.Context) {
				close(watchdogStarted)
				<-ctx.Done()
				close(watchdogExited)
			},
		})
	}()
	select {
	case <-scheduler.startedCh:
	case <-time.After(time.Second):
		t.Fatal("scheduler was not started")
	}
	select {
	case <-api.readyCh:
	case <-time.After(time.Second):
		t.Fatal("API was not marked ready")
	}
	select {
	case <-watchdogStarted:
	case <-time.After(time.Second):
		t.Fatal("watchdog was not started")
	}
	select {
	case <-notifier.readyCh:
	case <-time.After(time.Second):
		t.Fatal("systemd ready was not sent")
	}
	if !notifier.readyAfterAPIReady {
		t.Fatal("systemd ready was not sent after API readiness")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunInfrastructure() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunInfrastructure did not shut down")
	}
	select {
	case <-watchdogExited:
	default:
		t.Fatal("watchdog goroutine had not exited before shutdown completed")
	}
	if !api.shutdown || !store.closed || !notifier.stopping || !scheduler.stopped {
		t.Fatalf("shutdown state api=%t store=%t stopping=%t scheduler=%t", api.shutdown, store.closed, notifier.stopping, scheduler.stopped)
	}
}

func TestRunInfrastructureHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunInfrastructure(ctx, RuntimeOptions{}); err == nil {
		t.Fatal("RunInfrastructure() error = nil, want cancellation")
	}
}

func TestRunInfrastructureWithoutAPIDoesNotLogListening(t *testing.T) {
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler := &fakeScheduler{startedCh: make(chan struct{})}
	notifier := &simpleNotifier{readyCh: make(chan struct{})}
	store := &fakeStore{}
	done := make(chan error, 1)
	go func() {
		done <- RunInfrastructure(ctx, RuntimeOptions{
			Logger:          slog.New(slog.NewTextHandler(&logs, nil)),
			Store:           store,
			Scheduler:       scheduler,
			Notifier:        notifier,
			ShutdownTimeout: time.Second,
		})
	}()
	select {
	case <-scheduler.startedCh:
	case <-time.After(time.Second):
		t.Fatal("scheduler was not started")
	}
	select {
	case <-notifier.readyCh:
	case <-time.After(time.Second):
		t.Fatal("systemd ready was not sent")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunInfrastructure() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunInfrastructure did not shut down")
	}
	output := logs.String()
	if strings.Contains(output, "api listening") || strings.Contains(output, ":9587") {
		t.Fatalf("disabled API produced misleading log output: %s", output)
	}
	if !store.closed || !notifier.stopping || !scheduler.stopped {
		t.Fatalf("shutdown state store=%t stopping=%t scheduler=%t", store.closed, notifier.stopping, scheduler.stopped)
	}
}

func TestRunInfrastructureLogsAPIListeningWithBoundAddress(t *testing.T) {
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := &fakeAPI{addr: "127.0.0.1:34567", readyCh: make(chan struct{})}
	scheduler := &fakeScheduler{startedCh: make(chan struct{})}
	notifier := &fakeNotifier{api: api, readyCh: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- RunInfrastructure(ctx, RuntimeOptions{
			Logger:          slog.New(slog.NewTextHandler(&logs, nil)),
			API:             api,
			Scheduler:       scheduler,
			Notifier:        notifier,
			ShutdownTimeout: time.Second,
		})
	}()
	select {
	case <-api.readyCh:
	case <-time.After(time.Second):
		t.Fatal("API was not marked ready")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunInfrastructure() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunInfrastructure did not shut down")
	}
	output := logs.String()
	if !strings.Contains(output, "api listening") || !strings.Contains(output, "127.0.0.1:34567") {
		t.Fatalf("enabled API log output = %s", output)
	}
}

func TestRunInfrastructureRejectsAPIWithoutAddress(t *testing.T) {
	var logs bytes.Buffer
	api := &fakeAPI{emptyAddr: true}
	notifier := &simpleNotifier{readyCh: make(chan struct{})}
	store := &fakeStore{}
	err := RunInfrastructure(context.Background(), RuntimeOptions{
		Logger:          slog.New(slog.NewTextHandler(&logs, nil)),
		Store:           store,
		API:             api,
		Notifier:        notifier,
		ShutdownTimeout: time.Second,
	})
	if !errors.Is(err, errAPIAddressUnavailable) {
		t.Fatalf("RunInfrastructure() error = %v, want %v", err, errAPIAddressUnavailable)
	}
	if !api.started || !api.shutdown || !store.closed {
		t.Fatalf("cleanup state started=%t shutdown=%t store=%t", api.started, api.shutdown, store.closed)
	}
	if notifier.ready {
		t.Fatal("systemd ready was sent after API empty address failure")
	}
	if strings.Contains(logs.String(), "api listening") {
		t.Fatalf("empty API address produced listening log: %s", logs.String())
	}
}

func TestRunInfrastructureAPIStartFailureDoesNotLogListening(t *testing.T) {
	var logs bytes.Buffer
	startErr := errors.New("bind failed")
	api := &fakeAPI{startErr: startErr}
	notifier := &simpleNotifier{readyCh: make(chan struct{})}
	store := &fakeStore{}
	err := RunInfrastructure(context.Background(), RuntimeOptions{
		Logger:          slog.New(slog.NewTextHandler(&logs, nil)),
		Store:           store,
		API:             api,
		Notifier:        notifier,
		ShutdownTimeout: time.Second,
	})
	if !errors.Is(err, startErr) {
		t.Fatalf("RunInfrastructure() error = %v, want %v", err, startErr)
	}
	if !api.started || api.shutdown || !store.closed {
		t.Fatalf("cleanup state started=%t shutdown=%t store=%t", api.started, api.shutdown, store.closed)
	}
	if notifier.ready {
		t.Fatal("systemd ready was sent after API start failure")
	}
	if strings.Contains(logs.String(), "api listening") {
		t.Fatalf("failed API start produced listening log: %s", logs.String())
	}
}

type fakeAPI struct {
	mu        sync.Mutex
	started   bool
	ready     bool
	shutdown  bool
	readyCh   chan struct{}
	addr      string
	emptyAddr bool
	startErr  error
}

func (a *fakeAPI) Start(context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.started = true
	return a.startErr
}

func (a *fakeAPI) SetReady(ready bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ready = ready
	if ready && a.readyCh != nil {
		select {
		case <-a.readyCh:
		default:
			close(a.readyCh)
		}
	}
}

func (a *fakeAPI) Shutdown(context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.shutdown = true
	return nil
}

func (a *fakeAPI) Addr() string {
	if a.emptyAddr {
		return ""
	}
	if a.addr != "" {
		return a.addr
	}
	return "127.0.0.1:9587"
}

type fakeStore struct {
	closed bool
}

func (s *fakeStore) Close() error {
	s.closed = true
	return nil
}

type fakeScheduler struct {
	started   bool
	stopped   bool
	startedCh chan struct{}
}

func (s *fakeScheduler) Start(context.Context) error {
	s.started = true
	if s.startedCh != nil {
		close(s.startedCh)
	}
	return nil
}

func (s *fakeScheduler) Stop(context.Context) error {
	s.stopped = true
	return nil
}

type fakeNotifier struct {
	api                *fakeAPI
	readyAfterAPIReady bool
	stopping           bool
	readyCh            chan struct{}
}

func (n *fakeNotifier) Ready(context.Context) error {
	n.api.mu.Lock()
	defer n.api.mu.Unlock()
	n.readyAfterAPIReady = n.api.ready
	close(n.readyCh)
	return nil
}

func (n *fakeNotifier) Stopping(context.Context) error {
	n.stopping = true
	return nil
}

type simpleNotifier struct {
	ready    bool
	stopping bool
	readyCh  chan struct{}
}

func (n *simpleNotifier) Ready(context.Context) error {
	n.ready = true
	if n.readyCh != nil {
		close(n.readyCh)
	}
	return nil
}

func (n *simpleNotifier) Stopping(context.Context) error {
	n.stopping = true
	return nil
}
