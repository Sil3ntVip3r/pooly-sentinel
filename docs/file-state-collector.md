# File-State Collector

Task 5 collects metadata for explicitly configured filewatch targets only. It never scans the whole filesystem.

Collected metadata:

- normalized configured path
- target type
- existence
- file, directory, symlink, or other type
- owner UID/GID where available
- permission bits
- size
- modification time
- optional SHA-256 content hash for bounded regular files
- optional bounded directory-entry manifest hash
- manifest completeness, truncation, observed entry count, and entry limit

Security behavior:

- paths must be absolute
- configured target type must be `file`, `directory`, or `any`
- symlinks are detected with `Lstat`, rejected, and not followed
- regular files are opened with no-follow behavior on Linux/Unix hosts
- descriptor metadata is compared with the initial `Lstat` metadata before reading
- replaced files, type changes, and symlink substitution during collection are treated as incomplete source-change observations
- file hashing reads at most `max_file_bytes + 1` bytes and classifies the extra byte as oversized
- file contents are not emitted
- private-key-looking files are not hashed unless explicitly allowed
- directory manifests are one level only, deterministic, and entry-count bounded
- directory manifest inputs are sorted before limits are applied
- truncated manifests are marked incomplete/stale and are not persisted as trusted baselines
- permission-denied and disappearing files become observations, not panics

Directory manifest entries include name, type, permission mode, size, modification timestamp, and UID/GID where portably available. Entry contents are never read and directories are not traversed recursively.

The collector reports changed, unchanged, created, deleted, type-changed, permission-changed, owner-changed, modified, oversized, source-changed, symlink-rejected, type-mismatch, or manifest-truncated facts. It does not create incidents.
