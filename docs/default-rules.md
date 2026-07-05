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

Default rules are evaluated only by explicit rule-evaluation commands in Task 6. Task 7 can deliver notifications for resulting incidents only when explicitly invoked; there is still no production scheduler.
