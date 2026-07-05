# Linux Resource Collectors

Task 4 adds lightweight Linux resource collectors. They emit typed observations and metrics only. They do not send notifications, open incidents, evaluate thresholds, restart services, or start a production scheduler.

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

Task 4 implements parser-driven collectors for CPU, load average, memory, PSI pressure, filesystems, disk I/O, network interfaces, uptime, boot time, and boot-ID change detection.

Production targets Linux. On non-Linux platforms the manual collector command reports collectors as unsupported without panicking.
