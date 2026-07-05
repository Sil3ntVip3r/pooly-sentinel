# Notification Delivery

Task 7 adds single-cycle notification delivery for incident lifecycle events.

Notification delivery consumes existing incident records or incident transitions. It does not collect host data, evaluate rules, create incidents, resolve incidents, remediate services, or start a production scheduler.

## Events

Supported delivery events are:

- `opened`
- `escalated`
- `resolved`

Incident reopen transitions are treated as opened lifecycle events for delivery purposes.

## Flow

1. Load configured receivers.
2. Accept an incident or incident transition.
3. Determine the delivery event.
4. Render a safe payload from allowlisted incident fields.
5. Skip disabled or non-matching receivers.
6. Suppress duplicate successful deliveries.
7. Send through the receiver unless dry-run is active.
8. Persist each real delivery attempt.
9. Update `last_alerted` only after a successful delivery.

Failures are recorded as failed delivery attempts and do not mark the incident as alerted.

## CLI

```bash
pooly-agent notifications validate --config docs/config.example.yaml
pooly-agent notifications test --config docs/config.example.yaml --receiver local-webhook --dry-run
pooly-agent notifications send --config docs/config.example.yaml --incident <id> --dry-run
pooly-agent notifications deliveries --config docs/config.example.yaml --incident <id>
```

`notifications test` defaults to dry-run unless `--send` is explicitly provided.
