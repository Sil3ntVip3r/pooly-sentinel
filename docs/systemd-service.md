# systemd Service

The alpha unit lives at:

```text
systemd/pooly-sentinel-agent.service
```

Installed path:

```text
/etc/systemd/system/pooly-sentinel-agent.service
```

The unit uses:

- `Type=notify`
- `NotifyAccess=main`
- `Restart=on-failure`
- `RestartSec=5s`
- `StartLimitIntervalSec=10min`
- `StartLimitBurst=5`
- `WatchdogSec=60s`
- `TimeoutStartSec=30s`
- `TimeoutStopSec=30s`
- `StateDirectory=pooly-sentinel`
- `LogsDirectory=pooly-sentinel`
- `RuntimeDirectory=pooly-sentinel`
- `ConfigurationDirectory=pooly-sentinel`
- `EnvironmentFile=-/etc/pooly-sentinel/pooly-sentinel.env`
- journald stdout/stderr
- `NoNewPrivileges=true`
- `PrivateTmp=true`
- `ProtectSystem=full`
- `ProtectHome=read-only`
- explicit `ReadWritePaths` for Pooly Sentinel state, logs, and runtime directories

Startup validates config first:

```text
ExecStartPre=/usr/local/bin/pooly-agent check config --config /etc/pooly-sentinel/config.yaml
ExecStart=/usr/local/bin/pooly-agent run --config /etc/pooly-sentinel/config.yaml
```

Useful commands:

```bash
systemctl status pooly-sentinel-agent.service
journalctl -u pooly-sentinel-agent.service
systemctl start pooly-sentinel-agent.service
systemctl stop pooly-sentinel-agent.service
```

## Deferred Hardening

The alpha service does not yet run under a dedicated unprivileged user. Some collectors need broad read access to `/proc`, `/sys`, journald, SSH configuration, and security-sensitive files. Tighter privilege separation is deferred until a privileged-helper architecture exists.

The service does not perform remediation, SSH reloads, systemd restarts, updating, dashboard serving, public API exposure, or report delivery.
