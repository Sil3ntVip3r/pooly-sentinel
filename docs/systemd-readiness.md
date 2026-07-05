# systemd Readiness

Step 8 implements the systemd notification protocol used by the existing service template.

The agent reads `NOTIFY_SOCKET` and sends:

```text
READY=1
```

only after:

- configuration is loaded and validated
- logging is initialized
- SQLite storage is open and migrated
- the localhost API is listening when `api.enabled` is true
- the scheduler is initialized when `agent.scheduler.enabled` is true
- the run lifecycle has reached its ready point

If `NOTIFY_SOCKET` is absent, notification calls are safe no-ops.

When `agent.scheduler.run_on_start` is true, the run lifecycle waits for the first cycle to complete or fail safely before sending readiness. That first-cycle result is reflected in scheduler status.

## Watchdog

When `systemd.watchdog` is true and systemd provides a valid `WATCHDOG_USEC` environment value, the agent sends:

```text
WATCHDOG=1
```

at a bounded interval while the process is running. Watchdog notifications stop during shutdown.

Invalid notify sockets return errors to the caller for safe logging, but they do not crash the process.

## Boundaries

Readiness means configured runtime services are initialized. Scheduled monitoring is running only when `agent.scheduler.enabled` is true. Readiness does not mean remediation, updating, dashboard serving, public API exposure, report delivery, or automatic service changes are available.
