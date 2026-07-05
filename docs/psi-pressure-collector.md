# PSI Pressure Collector

Sources:

- `/proc/pressure/cpu`
- `/proc/pressure/memory`
- `/proc/pressure/io`

The parser accepts `some` and `full` lines with `avg10`, `avg60`, `avg300`, and `total`. A missing PSI file is reported as unsupported/unavailable when configured with `missing_is_ok`.

Average values are gauges. `total` values are cumulative counters in microseconds.

Metrics include:

- `pooly_pressure_cpu_some_avg10`
- `pooly_pressure_memory_some_avg10`
- `pooly_pressure_memory_full_avg10`
- `pooly_pressure_io_some_avg10`
- `pooly_pressure_io_full_avg10`
- matching `avg60`, `avg300`, and `_total_microseconds` metrics

Threshold interpretation is deferred to the future rule engine.
