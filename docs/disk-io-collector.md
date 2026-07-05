# Disk I/O Collector

Preferred source: `/sys/block/<device>/stat`.

Fallback parser: `/proc/diskstats`.

Sector counters are treated as 512-byte sectors. The collector emits cumulative counters and does not infer hardware failure from traffic counters alone.

Default exclusions:

- `loop*`
- `ram*`
- `fd*`
- `sr*`

The default discovery path reads whole devices from `/sys/block`, avoiding partition double counting. The diskstats parser also filters partitions when a whole disk is present. Device mapper, LVM, MD RAID, NVMe namespaces, and virtual disks are collected as bounded device names when discovered and not excluded.

Metrics include read/write bytes, reads/writes, read/write time, I/O time, weighted I/O time, and I/O in progress.
