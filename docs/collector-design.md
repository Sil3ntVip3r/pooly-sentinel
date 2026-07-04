# Collector Design

Collectors observe local state and emit typed observations. They do not own alerting, delivery, incident state, or storage policy.

## Pipeline

```text
raw source
  -> collector parser
  -> typed observation
  -> rule evaluation
  -> incident lifecycle change
  -> notification candidate
  -> grouping, dedupe, inhibition, silence, routing
  -> receiver delivery
  -> event, evidence, and history storage
```

## Collector Families

- `resources`: CPU, load, memory, pressure, filesystem, disk I/O, network, uptime
- `systemd`: critical services, failed units, agent self-health, unit drift
- `journal`: auth, service errors, kernel events, local evidence
- `ssh`: syntax, effective config, match context, ports, authorized keys summaries, permissions
- `filewatch`: watched sensitive paths, debounce, baseline, verification, overflow, rebuild
- `audit`: optional observe-only audit health and attribution

## Safety

Collectors should prefer structured local sources. Resource collectors should read `/proc` and `/sys` directly when available instead of shelling out.
