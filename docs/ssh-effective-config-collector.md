# SSH Effective-Config Collector

Step 11 collects effective SSH daemon settings for a fixed alpha profile matrix:

```text
profile=poolyadmin  user=poolyadmin
profile=admin2      user=pooly-sil3ntvip3r-admin
profile=root        user=root
```

Each profile runs `sshd -T -C` with deterministic local context:

```text
user=<profile-user>,host=localhost,addr=127.0.0.1,laddr=127.0.0.1,lport=<expected-port>
```

The expected port is taken from `ssh.expected.ports` when present. The collector does not restart, reload, or modify SSH. It does not read private keys or emit authorized-key contents.

Collected directives:

- `PermitRootLogin`
- `PasswordAuthentication`
- `KbdInteractiveAuthentication`
- `PermitEmptyPasswords`
- `PubkeyAuthentication`
- `StrictModes`
- `PermitUserEnvironment`

Directive names are normalized case-insensitively. Repeated directives are handled deterministically by keeping the last effective value.

Metrics report whether actual values match configured expected values with bounded labels: `directive=<safe-directive>` and `profile=poolyadmin|admin2|root`. Missing executables, timeout/cancellation, permission failure, output limits, malformed output, and syntax/non-zero failures are distinguished. Partial stdout after timeout is never treated as a successful effective configuration sample.

This is factual comparison only; alpha does not assign WARN, FAIL, CRITICAL, alerts, incidents, SSH reloads, or remediation from this collector by itself.
