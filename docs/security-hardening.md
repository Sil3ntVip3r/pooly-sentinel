# Security Hardening

Step 10 hardens alpha installation and validation without adding remediation.

## File Locations

- config: `/etc/pooly-sentinel/config.yaml`
- secret environment file: `/etc/pooly-sentinel/pooly-sentinel.env`
- state: `/var/lib/pooly-sentinel`
- logs: `/var/log/pooly-sentinel`
- binary: `/usr/local/bin/pooly-agent`
- service: `/etc/systemd/system/pooly-sentinel-agent.service`

## Permissions

- directories: `0750`
- installed config: `0640`
- secret environment file: `0600`
- installed binary: `0755`

The install script preserves existing configs unless `--force` is provided. The uninstall script preserves configs, env files, state, logs, and evidence by default.

## Runtime Boundaries

Collectors gather observations. Rules evaluate observations. Incidents manage lifecycle state. Notifications deliver only incident transitions. The scheduler coordinates those pieces only when explicitly enabled.

Not implemented in alpha:

- remediation or auto-repair
- updater
- dashboard
- public API
- report delivery
- new collectors
- new notification receivers
- remote fleet hub

Pooly Server Guard remains the fallback legacy guard and is not modified by this repository.
