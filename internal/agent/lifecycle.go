package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

func RunPlaceholder(ctx context.Context, logger *slog.Logger) error {
	if ctx == nil {
		return context.Canceled
	}
	if logger != nil {
		logger.InfoContext(ctx, "pooly-agent run placeholder started",
			slog.String("component", "agent"),
			slog.String("status", "production_monitoring_not_implemented"),
		)
	}
	<-ctx.Done()
	if logger != nil {
		logger.Info("pooly-agent shutdown requested",
			slog.String("component", "agent"),
			slog.String("reason", ctx.Err().Error()),
		)
	}
	return nil
}

type APIService interface {
	Start(ctx context.Context) error
	SetReady(ready bool)
	Shutdown(ctx context.Context) error
	Addr() string
}

type Store interface {
	Close() error
}

type Notifier interface {
	Ready(ctx context.Context) error
	Stopping(ctx context.Context) error
}

type SchedulerService interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type RuntimeOptions struct {
	Logger           *slog.Logger
	Store            Store
	API              APIService
	Scheduler        SchedulerService
	Notifier         Notifier
	ShutdownTimeout  time.Duration
	WatchdogEnabled  bool
	WatchdogInterval time.Duration
	RunWatchdog      func(ctx context.Context)
}

var errAPIAddressUnavailable = errors.New("api listener address unavailable after start")

func RunInfrastructure(ctx context.Context, opts RuntimeOptions) error {
	if ctx == nil {
		return context.Canceled
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 10 * time.Second
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.API != nil {
		if err := opts.API.Start(ctx); err != nil {
			closeStore(opts.Store, opts.Logger)
			return err
		}
		addr := opts.API.Addr()
		if addr == "" {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
			shutdownErr := opts.API.Shutdown(shutdownCtx)
			cancel()
			closeErr := closeStore(opts.Store, opts.Logger)
			return errors.Join(fmt.Errorf("api start: %w", errAPIAddressUnavailable), shutdownErr, closeErr)
		}
		if opts.Logger != nil {
			opts.Logger.InfoContext(ctx, "api listening",
				slog.String("component", "api"),
				slog.String("addr", addr),
			)
		}
	}
	if opts.Scheduler != nil {
		if err := opts.Scheduler.Start(ctx); err != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
			defer cancel()
			if opts.API != nil {
				_ = opts.API.Shutdown(shutdownCtx)
			}
			closeStore(opts.Store, opts.Logger)
			return err
		}
	}
	if opts.API != nil {
		opts.API.SetReady(true)
	}
	if opts.Notifier != nil {
		if err := opts.Notifier.Ready(ctx); err != nil && opts.Logger != nil {
			opts.Logger.WarnContext(ctx, "systemd readiness notification failed",
				slog.String("component", "systemdnotify"),
				slog.String("error_class", "notify"),
			)
		}
	}
	watchdogCtx, cancelWatchdog := context.WithCancel(ctx)
	var watchdogDone <-chan struct{}
	if opts.WatchdogEnabled && opts.RunWatchdog != nil {
		done := make(chan struct{})
		watchdogDone = done
		go func() {
			defer close(done)
			opts.RunWatchdog(watchdogCtx)
		}()
	}
	if opts.Logger != nil {
		opts.Logger.InfoContext(ctx, "pooly-agent run infrastructure ready",
			slog.String("component", "agent"),
			slog.String("status", "ready"),
		)
	}
	<-ctx.Done()
	cancelWatchdog()
	waitForWatchdog(watchdogDone, opts.ShutdownTimeout, opts.Logger)
	if opts.API != nil {
		opts.API.SetReady(false)
	}
	if opts.Scheduler != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
		_ = opts.Scheduler.Stop(shutdownCtx)
		cancel()
	}
	if opts.Notifier != nil {
		_ = opts.Notifier.Stopping(context.Background())
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
	defer cancel()
	var shutdownErr error
	if opts.API != nil {
		shutdownErr = errors.Join(shutdownErr, opts.API.Shutdown(shutdownCtx))
	}
	shutdownErr = errors.Join(shutdownErr, closeStore(opts.Store, opts.Logger))
	if opts.Logger != nil {
		opts.Logger.Info("pooly-agent shutdown complete",
			slog.String("component", "agent"),
			slog.String("reason", ctx.Err().Error()),
		)
	}
	return shutdownErr
}

func waitForWatchdog(done <-chan struct{}, timeout time.Duration, logger *slog.Logger) {
	if done == nil {
		return
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		if logger != nil {
			logger.Warn("systemd watchdog goroutine did not stop before timeout",
				slog.String("component", "systemdnotify"),
				slog.String("error_class", "timeout"),
			)
		}
	}
}

func closeStore(store Store, logger *slog.Logger) error {
	if store == nil {
		return nil
	}
	if err := store.Close(); err != nil {
		if logger != nil {
			logger.Warn("storage close failed",
				slog.String("component", "storage"),
				slog.String("error_class", "closed"),
			)
		}
		return err
	}
	return nil
}
