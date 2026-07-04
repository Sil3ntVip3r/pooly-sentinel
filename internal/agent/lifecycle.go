package agent

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
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
