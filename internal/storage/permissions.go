package storage

import "io/fs"

const (
	DirMode       fs.FileMode = 0o750
	FileMode      fs.FileMode = 0o640
	SecretEnvMode fs.FileMode = 0o600
)

const (
	DefaultDatabaseFile       = "state.db"
	DefaultCurrentMetricsFile = "metrics-current.json"
	DefaultMaxEventBytes      = 64 * 1024
)

func fileModeIsRestrictive(mode fs.FileMode) bool {
	perm := mode.Perm()
	return perm&0o007 == 0 && perm&0o020 == 0 && perm&0o400 != 0 && perm&0o200 != 0
}
