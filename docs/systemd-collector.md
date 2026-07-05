# systemd Collector

Task 5 collects factual systemd unit state for configured `systemd.critical_services`.

Data source:

- `systemctl show <unit> --no-pager --property=<allowlist>`

Allowed properties:

- `Id`
- `LoadState`
- `ActiveState`
- `SubState`
- `UnitFileState`
- `Result`
- `MainPID`
- `ExecMainCode`
- `ExecMainStatus`
- `NRestarts`
- `ActiveEnterTimestampMonotonic`

The collector does not parse human-oriented `systemctl status` output. It never restarts, reloads, enables, disables, masks, or edits a unit.

Command failures remain failures even when partial stdout is present. Timeout, cancellation, output-limit, missing-executable, permission, parse, and ordinary non-zero exits are separate observations. A non-zero exit may still represent a missing unit only when complete structured output contains `LoadState=not-found`; timeout, cancellation, and output-limit errors are never accepted as missing-unit success.

Metrics include factual state such as unit presence, active/failed/activating/deactivating state, restart count, main PID, exec main code/status, and active-enter monotonic seconds. The normalized unit name is the only systemd label.

Negative `MainPID`, `NRestarts`, or `ActiveEnterTimestampMonotonic` values are rejected as malformed.

Missing units, inactive units, failed units, activating units, and deactivating units are observations only. Task 6 rules may evaluate those observations, but the systemd collector itself does not assign severity, send alerts, create incidents, or remediate units.
