# Resource Daily Accumulation

Task 4 implements only lightweight current-day accumulation for future reports.

Tracked totals:

- daily network RX bytes per interface
- daily network TX bytes per interface
- daily disk read bytes per device
- daily disk write bytes per device

UTC day boundaries are used. Totals survive restart when persistence is enabled. Counter resets do not add negative deltas.

No historical retention, report delivery, or rollup database is implemented in Task 4.
