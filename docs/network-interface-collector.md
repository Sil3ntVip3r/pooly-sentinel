# Network Interface Collector

Source: `/sys/class/net`.

The collector reads interface counters, `operstate`, optional `carrier`, and `mtu`. MAC addresses are not emitted as metrics or labels.

Default exclusions:

- `lo`
- `docker*`
- `veth*`
- `br-*`

Tunnel interfaces such as `tailscale*` and `wg*` are not excluded unconditionally.

Metrics include RX/TX bytes, packets, errors, drops, interface up state, carrier state, MTU, and current UTC-day RX/TX byte totals.

Missing `carrier` is represented as unknown, not automatically down. Counter resets refresh baseline state and never emit negative deltas.
