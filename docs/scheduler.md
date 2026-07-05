# Scheduler

Step 9 adds the disabled-by-default production monitoring scheduler and single-agent run loop.

The scheduler coordinates existing components only:

1. collect typed observations from enabled collectors
2. evaluate configured rules
3. let the incident engine update local incident state
4. deliver notifications for incident transitions when notification config allows it
5. persist safe scheduler status

It does not add collectors, rule semantics, notification receivers, remediation, updating, dashboard work, public API exposure, or report delivery.

## Configuration

```yaml
agent:
  scheduler:
    enabled: false
    interval: 60s
    run_on_start: false
    cycle_timeout: 45s
    max_consecutive_failures: 5
```

The scheduler is disabled by default. Validation rejects non-positive durations, cycle timeouts greater than or equal to the interval, unreasonably short or large intervals, and negative failure limits.

## Cycle Pipeline

Each cycle is deterministic:

1. mark the attempt start time
2. collect observations from enabled collectors
3. preserve unsupported, stale, first-baseline, reset, timeout, parse, permission, and source-missing states as typed observations
4. evaluate rules through the existing rule engine
5. apply incident lifecycle changes through the existing incident engine
6. deliver notifications only for incident transitions returned by the rule/incident pipeline
7. persist safe scheduler status

Unsupported collectors and counter-reset or first-baseline observations are not cycle failures by default. Supported collector failures are passed to rule evaluation and then mark the scheduler cycle as failed. Failed rule evaluation, failed notification delivery, cancellation, timeout, panic recovery, or scheduler-status persistence failure marks the cycle as failed and does not update the last successful cycle time.

## CLI

```bash
pooly-agent scheduler status --config docs/config.example.yaml
pooly-agent scheduler run-once --config docs/config.example.yaml --dry-run
pooly-agent scheduler run-once --config docs/config.example.yaml --json --dry-run
```

`run-once` defaults to dry-run storage. Dry-run uses a temporary database and dry-run notifications, so it does not persist collector baselines, rule state, incidents, notification deliveries, or scheduler status to configured production storage.

Use `--persist` only when intentionally running a real single cycle against configured local storage.

## Status

Scheduler status is exposed through `pooly-agent scheduler status`, `GET /status`, and report summaries. Safe fields include enabled/running state, interval, last attempt time, last successful cycle time, last cycle duration, safe error class and summary, cycle counts, consecutive failures, and whether a cycle is currently active.

Status does not include secrets, raw command output, raw journal messages, webhook URLs, tokens, usernames, source IPs, MAC addresses, boot UUIDs, private keys, file contents, or private evidence.

## Readiness

`pooly-agent run` initializes config, logging, storage, the localhost API when enabled, and the scheduler when enabled before sending systemd `READY=1`. If `run_on_start` is enabled, startup waits for that first cycle to complete or fail before readiness; a failed run-on-start cycle is recorded but does not crash the process.

## Known Limitations

The scheduler is a local single-agent loop only. It does not implement report delivery, retention cleanup, remediation, updater behavior, dashboard UI, public API exposure, new notification receivers, or external SaaS integrations.
