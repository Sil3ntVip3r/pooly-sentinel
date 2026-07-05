# Filesystem Collector

Linux source: `statfs` via `golang.org/x/sys/unix`.

The collector checks configured mounts only. It does not recursively walk filesystems, write probe files, cross mount boundaries, or discover arbitrary paths for labels.

Primary service-facing capacity:

```text
used_for_service_ratio = 1 - available_blocks / total_blocks
```

Metrics:

- `pooly_filesystem_size_bytes`
- `pooly_filesystem_available_bytes`
- `pooly_filesystem_free_bytes`
- `pooly_filesystem_used_bytes`
- `pooly_filesystem_used_ratio`
- `pooly_filesystem_inodes_total`
- `pooly_filesystem_inodes_free`
- `pooly_filesystem_inodes_used_ratio`
- `pooly_filesystem_readonly`

Only the configured mount label is used.
