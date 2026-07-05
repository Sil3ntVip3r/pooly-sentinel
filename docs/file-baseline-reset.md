# File Baseline And Reset Behavior

Filewatch persistence uses the existing `collector_state.state_json` field.

Behavior:

- first persisted run records a baseline
- later persisted runs compare current metadata to the baseline
- dry-run mode reads state but does not update it
- corrupt baseline state marks the observation stale/reset and records a fresh baseline when persistence is enabled
- failed, oversized, type-mismatched, source-changed, symlink, canceled, or truncated-manifest samples do not overwrite a valid baseline
- context cancellation before state update prevents baseline writes
- storage write failures are returned as state errors

Only metadata and optional hashes are persisted. File contents, private keys, authorized-key contents, webhook URLs, tokens, and passwords are not persisted.

Task 5 does not implement event-driven filesystem watching or a production scheduler. It supports deterministic manual and future periodic verification.
