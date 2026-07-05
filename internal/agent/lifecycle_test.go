package agent

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRunInfrastructureReadinessAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := &fakeAPI{readyCh: make(chan struct{})}
	notifier := &fakeNotifier{api: api, readyCh: make(chan struct{})}
	store := &fakeStore{}
	watchdogStarted := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- RunInfrastructure(ctx, RuntimeOptions{
			Store:           store,
			API:             api,
			Notifier:        notifier,
			ShutdownTimeout: time.Second,
			WatchdogEnabled: true,
			RunWatchdog: func(ctx context.Context) {
				close(watchdogStarted)
				<-ctx.Done()
			},
		})
	}()
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
	if !api.shutdown || !store.closed || !notifier.stopping {
		t.Fatalf("shutdown state api=%t store=%t stopping=%t", api.shutdown, store.closed, notifier.stopping)
	}
}

func TestRunInfrastructureHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunInfrastructure(ctx, RuntimeOptions{}); err == nil {
		t.Fatal("RunInfrastructure() error = nil, want cancellation")
	}
}

type fakeAPI struct {
	mu       sync.Mutex
	started  bool
	ready    bool
	shutdown bool
	readyCh  chan struct{}
}

func (a *fakeAPI) Start(context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.started = true
	return nil
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

func (a *fakeAPI) Addr() string { return "127.0.0.1:9587" }

type fakeStore struct {
	closed bool
}

func (s *fakeStore) Close() error {
	s.closed = true
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
