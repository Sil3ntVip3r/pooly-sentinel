# Agent Design

This file reserves the high-level agent design notes requested by the build brief. The current repository state is Task 1 only, so this document describes intended boundaries rather than implemented behavior.

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

The agent should send `READY=1` only after config is loaded, storage is ready, collectors are initialized, the notifier is initialized, the local API is bound, and the first self-check is complete.

## Non-Goals

- No hub or dashboard in the initial agent.
- No public API by default.
- No paid notification dependency.
- No automatic repair during alpha.
