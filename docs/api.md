# Localhost API

Step 8 adds a read-only HTTP API for local diagnostics. Step 9 adds safe scheduler status to the existing status and report paths.

The API is disabled by default and binds to `127.0.0.1:9587` when enabled. Configuration validation rejects non-loopback bind addresses unless `api.allow_non_loopback` is explicitly set. The alpha agent has no dashboard and no public API by default.

## Endpoints

- `GET /healthz`
- `GET /readyz`
- `GET /status`
- `GET /incidents`
- `GET /incidents/{id}`
- `GET /notifications/deliveries`
- `GET /reports/summary`
- `GET /metrics/status`

All endpoints return JSON. There are no write endpoints.

`GET /status` includes safe scheduler fields when the run lifecycle provides them: enabled/running state, interval, last attempt time, last successful cycle time, duration, error class and summary, cycle counts, consecutive failures, and active-cycle state.

## Safety

Responses are redacted before they leave the API. They do not include webhook URLs, tokens, passwords, private keys, authorization headers, raw journal messages, raw command output, file contents, source IPs, MAC addresses, boot UUIDs, usernames, or arbitrary unredacted error text.

Incident responses include only safe persisted incident fields. Evidence paths are included only when they remain local-path-safe after redaction checks; evidence contents are never read or returned.

Notification delivery responses include delivery IDs, incident IDs, receiver IDs, status, attempt counts, timestamps, and redacted error summaries. Destination URLs and headers are never returned.

## Boundaries

The API does not run collectors, evaluate rules, create incidents, send notifications, remediate services, update the host, deliver reports, or start scheduler cycles.
