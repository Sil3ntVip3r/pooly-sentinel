# Agent Design

This file reserves high-level agent design notes requested by the build brief. The repository now includes the core foundation, storage foundation, one-shot collectors, rule evaluation, local incident lifecycle persistence, single-cycle notification delivery, read-only localhost API serving, local report preview, and systemd readiness/watchdog wiring. Production scheduling, report delivery, dashboard work, and remediation remain future work.

## Responsibilities

The `pooly-agent` process will coordinate:

- config loading and validation
- storage initialization
- collector scheduling
- rule evaluation
- incident lifecycle updates
- notification routing and delivery
- localhost API endpoints
- retention cleanup
- daily report generation
- systemd readiness and watchdog signaling

## Startup Guardrail

The agent sends `READY=1` only after config is loaded, logging is initialized, storage is open and migrated, the local API is bound when enabled, and the run lifecycle reaches its ready point. Production collector scheduling is still deferred.

## Non-Goals

- No hub or dashboard in the initial agent.
- No public API by default.
- No paid notification dependency.
- No automatic repair during alpha.
