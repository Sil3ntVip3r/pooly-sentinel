# Storage and Retention

Pooly Sentinel will keep local state, event history, evidence, and rollups.

## Planned Paths

- State database: `/var/lib/pooly-sentinel/state.db`
- Current metrics: `/var/lib/pooly-sentinel/metrics-current.json`
- Events: `/var/log/pooly-sentinel/events/*.jsonl`
- Open incident evidence: `/var/log/pooly-sentinel/incidents/open/<incident_id>/`
- Resolved incident evidence: `/var/log/pooly-sentinel/incidents/resolved/<incident_id>/`
- Rollups: `/var/lib/pooly-sentinel/rollups/`

## Retention Principles

- Current state is never deleted.
- Open incidents are never deleted.
- Security evidence is protected.
- Retention cleanup must never delete active incident evidence.

## Status

Task 1 creates storage package placeholders only. No files are written under system state or log directories.
