# Resource Counter State

Cumulative counters use a shared reset-safe policy:

```text
current >= previous:
  delta = current - previous

current < previous:
  mark reset
  delta unavailable
  never emit negative usage
```

Baseline state is persisted through Task 3 `collector_state.state_json` only when collection is explicitly run with persistence enabled. Manual diagnostics default to dry-run/no-persist.

State is updated only after a sample parses and validates successfully.
