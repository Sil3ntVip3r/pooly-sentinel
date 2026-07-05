# Run Lifecycle

Step 8 changed `pooly-agent run --config <path>` from a placeholder wait loop into infrastructure wiring. Step 9 adds optional scheduler initialization to that lifecycle.

Startup order:

1. load and validate configuration
2. initialize redacting structured logging
3. open and migrate SQLite storage
4. start the localhost API when enabled
5. initialize and start the scheduler when `agent.scheduler.enabled` is true
6. mark the API ready
7. send `READY=1` to systemd when configured and available
8. send watchdog notifications when systemd config and environment allow it
9. wait for `SIGINT`, `SIGTERM`, or context cancellation

Shutdown order:

1. mark readiness false
2. stop watchdog notifications
3. stop the scheduler
4. send `STOPPING=1` when systemd notification is available
5. gracefully shut down the API
6. close storage

If `agent.scheduler.run_on_start` is true, startup runs one scheduler cycle before readiness is sent. A failed run-on-start cycle records safe scheduler status and logs a redacted warning; it does not crash the process.

## Boundaries

The run lifecycle starts scheduled collection only when `agent.scheduler.enabled` is explicitly true. It does not deliver reports, remediate services, update the host, serve a dashboard, expose a public API, add collectors, add notification receivers, or perform automatic service changes.
