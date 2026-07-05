# Task 5 Scope Boundaries

Task 5 collectors gather facts only.

Implemented:

- systemd service-state observations
- journald bounded JSON parsing and cursor state
- SSH effective configuration observations
- SSH listening-port observations
- security-sensitive file metadata observations
- one-shot manual CLI commands

Not implemented:

- rule engine
- threshold evaluation
- WARN, FAIL, or CRITICAL policy severity
- incident creation or lifecycle transitions
- notification delivery
- report generation or delivery
- production scheduler or monitoring loop
- API server
- systemd readiness or watchdog
- service installation
- updater
- dashboard
- audit collector
- automatic repair
- SSH reload or restart
- systemd unit restart
