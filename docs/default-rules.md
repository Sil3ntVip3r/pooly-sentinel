# Default Rules

The example configuration includes conservative local rules for:

- sustained CPU usage
- memory availability
- filesystem capacity
- inode capacity
- required network interface state
- required systemd service failure
- forbidden SSH listener
- SSH hardening drift
- kernel OOM events

Host-specific rules such as required interfaces, required services, forbidden SSH listener checks, SSH hardening drift, and kernel OOM event handling are disabled in the example until the operator chooses safe local targets.

Default rules can be evaluated by explicit rule-test commands or by the Step 9 scheduler when `agent.scheduler.enabled` is explicitly true. The scheduler remains disabled by default.
