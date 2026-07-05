# Rule Engine

Task 6 adds deterministic rule evaluation over typed collector observations.

Rules consume observations from the resource, systemd, journal, SSH, and file-state collectors. Collectors continue to gather facts only and do not evaluate thresholds, assign WARN/FAIL/CRITICAL severity, send notifications, or create incidents.

## Behavior

- Rules are enabled or disabled by configuration.
- A rule selects a collector plus a metric or event category.
- Optional targets constrain evaluation to a safe bounded target such as `system`, a mount label, an interface, a unit, a directive, or a port.
- Thresholds support warning, failure, and critical levels.
- Sustained durations prevent a single sample from opening an incident.
- Recovery duration prevents brief healthy samples from resolving an active incident too quickly.
- Missing, stale, unsupported, parse-error, timeout, counter-reset, and first-baseline observations are handled by explicit policy.

## State Machine

The implemented rule states are:

- `OK`
- `PENDING_WARN`
- `WARN`
- `PENDING_FAIL`
- `FAIL`
- `CRITICAL`
- `RECOVERING`
- `RECOVERED`
- `STALE`
- `UNKNOWN`

Pending state resets to `OK` when the condition clears before its configured duration. Counter resets and first-baseline samples do not create incidents.

## Deferred

Task 6 does not add production scheduling, notification delivery, remediation, API serving, dashboard functionality, or systemd readiness.
