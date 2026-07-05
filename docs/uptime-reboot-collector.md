# Uptime and Reboot Collector

Sources:

- `/proc/uptime`
- `/proc/stat` `btime`
- `/proc/sys/kernel/random/boot_id`

The boot ID is stored only as minimal local collector state. It is not emitted as a metric label.

Metrics:

- `pooly_system_uptime_seconds`
- `pooly_system_boot_time_timestamp_seconds`
- `pooly_system_boot_id_changed`

The first observation records a baseline without reporting a false reboot. A later boot-ID change is emitted as an observation metric only; incident handling is deferred.
