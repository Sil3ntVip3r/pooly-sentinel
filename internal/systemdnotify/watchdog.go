package systemdnotify

import (
	"context"
	"log/slog"
	"time"
)

func RunWatchdog(ctx context.Context, client Client, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = client.WatchdogInterval(30 * time.Second)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Watchdog(ctx); err != nil && logger != nil {
				logger.WarnContext(ctx, "systemd watchdog notification failed",
					slog.String("component", "systemdnotify"),
					slog.String("error_class", "notify"),
				)
			}
		}
	}
}
