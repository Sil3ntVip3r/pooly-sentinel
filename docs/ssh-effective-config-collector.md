# SSH Effective-Config Collector

Task 5 collects effective SSH daemon settings with:

```text
sshd -T -C user=pooly-sentinel,host=localhost,addr=127.0.0.1
```

The connection context is deterministic and local. It is used only to make `sshd -T` produce effective settings without changing SSH state.

Collected directives:

- `PermitRootLogin`
- `PasswordAuthentication`
- `KbdInteractiveAuthentication`
- `PermitEmptyPasswords`
- `PubkeyAuthentication`
- `StrictModes`
- `PermitUserEnvironment`

Directive names are normalized case-insensitively. Repeated directives are handled deterministically by keeping the last effective value.

Missing executables, timeout/cancellation, permission failure, output limits, and syntax/non-zero failures are distinguished. Partial stdout after timeout is never treated as a successful effective configuration sample.

Metrics report whether actual values match configured expected values. This is factual comparison only; Task 5 does not assign WARN, FAIL, CRITICAL, alerts, incidents, or remediation.
