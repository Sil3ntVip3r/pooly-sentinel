# Security Hardening

Step 11 hardens alpha behavior without adding remediation, an updater, a dashboard, public API exposure, report delivery, new notification receivers, broad collectors, real deployment, or Pooly Server Guard changes.

## File Locations

- config: `/etc/pooly-sentinel/config.yaml`
- secret environment file: `/etc/pooly-sentinel/pooly-sentinel.env`
- state: `/var/lib/pooly-sentinel`
- logs: `/var/log/pooly-sentinel`
- binary: `/usr/local/bin/pooly-agent`
- service: `/etc/systemd/system/pooly-sentinel-agent.service`

## Permissions And Paths

- directories: `0750`
- installed config: `0640`
- secret environment file: `0600`
- installed binary: `0755`
- SQLite and JSONL state/log files are created with restrictive permissions and reject symlink or non-regular final paths.
- Atomic state/evidence writes fsync the temporary file, close it, rename it, then fsync the containing directory and return directory-sync errors.
- Installer and uninstaller path overrides must be absolute, non-empty, free of control characters, and not broad system paths such as `/`, `/etc`, `/usr`, `/var`, `/bin`, `/sbin`, `/lib`, or `/lib64`. Purge paths have extra broad-path guards.

The install script preserves existing configs unless `--force` is provided. The uninstall script preserves configs, env files, state, logs, and evidence by default. Service enable/start and destructive removals require explicit flags.

## Runtime Boundaries

Collectors gather observations. Rules evaluate observations. Incidents manage lifecycle state. Notifications deliver only incident transitions. The scheduler coordinates those pieces only when explicitly enabled.

Hardening added in Step 11:

- generic webhook delivery does not follow redirects; `3xx` responses fail delivery and redirect targets are not included in summaries
- journald cursor replay uses `--after-cursor` without `--lines=N`; cursors advance only after complete successful parsing and processing
- SSH effective config checks fixed profiles `poolyadmin`, `admin2`, and `root` with safe `profile` labels and configured `lport` context
- command timeouts/cancellations kill the Unix process group where supported and keep timeout, cancellation, and output-limit classes distinct
- unexpected API `Serve` errors mark readiness false and appear as redacted `/status` errors
- watchdog shutdown cancels and waits for the watchdog goroutine, with a safe timeout warning if it does not exit
- API and notification evidence paths share one sanitizer and never return evidence contents

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
