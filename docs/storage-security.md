# Storage Security

Storage is local-only in Task 3. Nothing is sent externally.

## Secret Handling

Storage errors are wrapped with operation context and redacted before formatting. File writers redact sensitive keys and secret-shaped strings before persistence.

Sensitive values must not be placed in configuration. Secret environment files are documented as `0600`, but Task 3 does not create or manage them.

## Atomic Current State

Current-state JSON writes use a temporary file in the destination directory, flush and close it, then atomically rename it into place. This prevents partially written final files.

## JSONL Events

JSONL event writes are serialized in-process. The Task 3 policy rejects oversized events instead of truncating them, so callers receive a clear error and no ambiguous partial event is written.

## Evidence Files

Evidence writes are local only. Incident IDs are sanitized for path segments, filenames must be plain relative names, absolute filenames are rejected, and path traversal is rejected.

Evidence directories and files use restrictive permissions. Evidence is written atomically and secrets are redacted before persistence.

## What Task 3 Does Not Do

- no retention deletion
- no deletion of open incident evidence
- no production collectors
- no notification delivery
- no rule or incident lifecycle processing
- no systemd readiness or watchdog integration
