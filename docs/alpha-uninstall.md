# Alpha Uninstall

The uninstall helper is intentionally conservative. By default it prints preserved paths and removes nothing.

Preview:

```bash
scripts/uninstall.sh --dry-run
```

Stop and disable the service explicitly:

```bash
sudo scripts/uninstall.sh --stop-service --disable-service
```

Remove installed binary and service file explicitly:

```bash
sudo scripts/uninstall.sh --remove-binary --remove-service
```

State, logs, config, secret environment files, and incident evidence are preserved by default:

- `/etc/pooly-sentinel/config.yaml`
- `/etc/pooly-sentinel/pooly-sentinel.env`
- `/var/lib/pooly-sentinel`
- `/var/log/pooly-sentinel`

State or logs can be purged only with explicit confirmation flags:

```bash
sudo scripts/uninstall.sh --purge-state --confirm-purge-state
sudo scripts/uninstall.sh --purge-logs --confirm-purge-logs
```

Do not purge state until incidents and evidence have been reviewed.

## Rollback

Rollback for alpha is service-level: stop and disable `pooly-sentinel-agent.service`, then continue using the existing Bash-based Pooly Server Guard. This repository does not modify `pooly-server-guard`.
