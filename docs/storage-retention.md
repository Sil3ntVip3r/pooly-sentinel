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

Task 3 implements the storage foundation only:

- SQLite state database creation and migrations
- typed repository methods for metadata, collector state, incidents, and notification delivery history
- atomic current-state JSON writing
- append-only JSONL event writing
- local incident evidence writing
- storage-focused doctor checks

Task 3 does not implement retention cleanup, resource aggregation, incident lifecycle processing, notification delivery, or production monitoring loops.

## Permissions

Intended defaults:

- directories: `0750`
- ordinary state and log files: `0640`
- secret environment files: `0600`

The implementation creates files with restrictive modes and respects the process umask.
