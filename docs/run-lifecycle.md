# Run Lifecycle

Step 8 changes `pooly-agent run --config <path>` from a placeholder wait loop into infrastructure wiring.

Startup order:

1. load and validate configuration
2. initialize redacting structured logging
3. open and migrate SQLite storage
4. start the localhost API when enabled
5. mark the API ready
6. send `READY=1` to systemd when configured and available
7. send watchdog notifications when systemd config and environment allow it
8. wait for `SIGINT`, `SIGTERM`, or context cancellation

Shutdown order:

1. mark readiness false
2. stop watchdog notifications
3. send `STOPPING=1` when systemd notification is available
4. gracefully shut down the API
5. close storage

## Boundaries

The run lifecycle does not start a production collector scheduler. It does not automatically run collectors, evaluate rules, create incidents, send notifications, deliver reports, remediate services, update the host, or serve a dashboard.
