# Linux Resource Collectors

Phase 1A adds lightweight resource monitoring. These collectors are intended for early warning and local daily reporting, not dashboard-scale telemetry.

## Sources

- CPU and load: `/proc/stat`, `/proc/loadavg`
- Memory: `/proc/meminfo`
- Pressure: `/proc/pressure/*` when available
- Filesystems: Go statfs-style calls
- Disk I/O: `/proc/diskstats`, `/sys/block/<device>/stat`
- Network: `/sys/class/net/<iface>/statistics/*`, `operstate`, `carrier`
- Uptime and reboot: `/proc/uptime`, `/proc/stat` boot time, journald boot ID

## Design Rules

- Calculate CPU from deltas, not a single sample.
- Use `MemAvailable`, not only `MemFree`.
- Missing PSI files should not fail the agent.
- Monitor inodes as well as bytes.
- Handle counter resets without reporting negative usage.
- Treat iowait as supporting evidence, not standalone failure truth.

## Status

No resource collector implementation exists in Task 1.
