# Alpha Install

Pooly Sentinel alpha install is conservative. It installs the Go agent beside the existing Pooly Server Guard; it does not modify `pooly-server-guard`, auto-repair services, start the scheduler by default, or send notifications.

Build first:

```bash
CGO_ENABLED=0 go build ./cmd/pooly-agent
```

Preview the install:

```bash
scripts/install.sh --dry-run
```

Install files without starting the service:

```bash
sudo scripts/install.sh
```

The installer creates:

- `/usr/local/bin/pooly-agent`
- `/etc/pooly-sentinel/config.yaml`
- `/etc/pooly-sentinel/pooly-sentinel.env`
- `/var/lib/pooly-sentinel`
- `/var/log/pooly-sentinel`
- `/etc/systemd/system/pooly-sentinel-agent.service`

Permissions are conservative: directories use `0750`, installed config uses `0640`, and the environment file uses `0600`.

Enable or start only when explicitly ready:

```bash
sudo scripts/install.sh --enable-service
sudo systemctl start pooly-sentinel-agent.service
```

or:

```bash
sudo scripts/install.sh --enable-service --start-service
```

## Configuration

The installer installs `docs/config.example.yaml` only when no real config exists, unless `--force` is provided. The example contains no webhook URLs, tokens, passwords, private keys, or secret values. Put secret environment values in `/etc/pooly-sentinel/pooly-sentinel.env` out of band.

Scheduler remains disabled by default:

```yaml
agent:
  scheduler:
    enabled: false
```

Validate after editing:

```bash
pooly-agent check config --config /etc/pooly-sentinel/config.yaml
pooly-agent doctor --config /etc/pooly-sentinel/config.yaml
```

## Rollback

Pooly Server Guard remains the fallback legacy guard. To roll back the alpha agent, stop Pooly Sentinel and leave state/config in place for inspection:

```bash
sudo systemctl stop pooly-sentinel-agent.service
sudo systemctl disable pooly-sentinel-agent.service
```
