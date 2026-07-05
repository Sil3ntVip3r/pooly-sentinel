# Memory Collector

Source: `/proc/meminfo`.

Required fields include `MemTotal`, `MemAvailable`, `MemFree`, `Buffers`, `Cached`, swap, dirty/writeback, slab, kernel stack, and page table fields. Kernel `kB` values are converted to bytes.

Primary formulas:

```text
memory_available_ratio = MemAvailable / MemTotal
memory_used_ratio = 1 - memory_available_ratio
swap_used_ratio = 0 when SwapTotal is zero, otherwise 1 - SwapFree / SwapTotal
```

`MemAvailable` is the primary availability signal. Page cache is not treated as automatically wasted memory.

Metrics include memory total, available, free, used ratio, available ratio, swap totals, dirty/writeback bytes, and slab bytes.
