# journald Collectors

Task 5 implements one-shot collectors for:

- `journal.auth`
- `journal.services`
- `journal.kernel`

Data source:

- `journalctl --no-pager --output=json`
- bounded `--lines`
- persisted `--after-cursor` when persistence is enabled
- stream-specific filters for auth, service, and kernel records

Each stream stores its cursor separately in collector state. The first persisted run records a bounded baseline cursor. Later persisted runs read records after that cursor. Cursor corruption, invalidation, rotation, or vacuuming marks the observation stale/reset and records a fresh bounded baseline instead of replaying an unbounded journal.

Cursors are saved only after successful complete processing. Timeout, malformed JSON, output-limit failures, and record truncation do not advance the cursor. Dry-run mode never persists cursor state.

Records are normalized into safe categories such as authentication failure, authentication success, invalid user, sudo failure, service start/stop/failure/restart, kernel OOM, storage, network, or generic journal event. These are event categories, not alert severity.

Task 5 does not emit raw journal dumps, full `MESSAGE` fields, credentials, tokens, command lines, or environment dumps.
