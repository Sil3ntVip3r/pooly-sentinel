# CPU Collector

Source: `/proc/stat`.

The parser reads aggregate `cpu` and per-CPU `cpuN` lines. CPU usage is calculated from two samples:

```text
total = user + nice + system + idle + iowait + irq + softirq + steal
busy = total - idle - iowait
used_ratio = busy_delta / total_delta
```

`guest` and `guest_nice` are not added to totals because Linux already includes them in `user` and `nice`.

Metrics:

- `pooly_cpu_used_ratio`
- `pooly_cpu_iowait_ratio`
- `pooly_cpu_steal_ratio`
- `pooly_cpu_count`

The first sample records a baseline and does not emit a false usage spike. Counter resets mark the observation stale and refresh the baseline without emitting negative deltas.
