# Rule Configuration

Rules live under the top-level `rules` list in YAML.

```yaml
rules:
  - id: memory-available-low
    enabled: true
    collector: resources
    metric: pooly_memory_available_ratio
    target: system
    warn:
      operator: less_than
      value: 0.15
      for: 10m
    fail:
      operator: less_than
      value: 0.08
      for: 10m
    critical:
      operator: less_than
      value: 0.04
      for: 5m
    recover_for: 5m
    missing_data: stale
    stale_data: stale
```

Supported operators are:

- `greater_than`
- `greater_than_or_equal`
- `less_than`
- `less_than_or_equal`
- `equal`
- `not_equal`
- `boolean_true`
- `boolean_false`
- `state_match`
- `event_category_match`

Policies for missing and stale data are `ignore`, `stale`, `warn`, and `fail`. Unsupported collectors are not treated as failures by default.

Validation rejects duplicate rule IDs, unknown operators, invalid metric names, unsafe targets, negative durations, excessive summaries, excessive rule counts, and secret-bearing text.

Rule configuration represents threshold values. The collectors themselves do not evaluate those thresholds.
