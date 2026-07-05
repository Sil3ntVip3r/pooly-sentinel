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
- the run lifecycle has reached its ready point

If `NOTIFY_SOCKET` is absent, notification calls are safe no-ops.

## Watchdog

When `systemd.watchdog` is true and systemd provides a valid `WATCHDOG_USEC` environment value, the agent sends:

```text
WATCHDOG=1
```

at a bounded interval while the process is running. Watchdog notifications stop during shutdown.

Invalid notify sockets return errors to the caller for safe logging, but they do not crash the process.

## Boundaries

Readiness does not mean production monitoring is running. Step 8 does not start collector scheduling, rule evaluation loops, notification loops, remediation, updating, or a dashboard.
