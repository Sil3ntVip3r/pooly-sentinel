package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

type Migration struct {
	Version    int
	Name       string
	Statements []string
}

var defaultMigrations = []Migration{
	{
		Version: 1,
		Name:    "metadata",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS metadata (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		},
	},
	{
		Version: 2,
		Name:    "collector_state",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS collector_state (
				collector TEXT NOT NULL,
				target TEXT NOT NULL,
				status TEXT NOT NULL,
				state_json TEXT NOT NULL,
				last_attempt_at TEXT,
				last_success_at TEXT,
				last_error_class TEXT,
				last_error_summary TEXT,
				updated_at TEXT NOT NULL,
				PRIMARY KEY (collector, target)
			)`,
		},
	},
	{
		Version: 3,
		Name:    "incidents",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS incidents (
				id TEXT PRIMARY KEY,
				node_id TEXT NOT NULL,
				type TEXT NOT NULL,
				target TEXT NOT NULL,
				condition TEXT NOT NULL,
				severity TEXT NOT NULL,
				status TEXT NOT NULL,
				summary TEXT NOT NULL,
				first_seen TEXT NOT NULL,
				last_seen TEXT NOT NULL,
				last_alerted TEXT,
				occurrence_count INTEGER NOT NULL CHECK (occurrence_count >= 0),
				evidence_path TEXT,
				resolved_at TEXT,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_incidents_status_updated_at ON incidents(status, updated_at)`,
			`CREATE INDEX IF NOT EXISTS idx_incidents_node_type_target ON incidents(node_id, type, target)`,
		},
	},
	{
		Version: 4,
		Name:    "notification_deliveries",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS notification_deliveries (
				id TEXT PRIMARY KEY,
				incident_id TEXT NOT NULL,
				receiver TEXT NOT NULL,
				cost_class TEXT NOT NULL,
				status TEXT NOT NULL,
				attempt INTEGER NOT NULL CHECK (attempt >= 1),
				attempted_at TEXT NOT NULL,
				delivered_at TEXT,
				error_class TEXT,
				error_summary TEXT,
				FOREIGN KEY (incident_id) REFERENCES incidents(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_incident_attempted ON notification_deliveries(incident_id, attempted_at)`,
		},
	},
	{
		Version: 5,
		Name:    "rollup_metadata",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS rollup_metadata (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		},
	},
}

func LatestSchemaVersion() int {
	return latestMigrationVersion(defaultMigrations)
}

func runMigrations(ctx context.Context, db *sql.DB, migrations []Migration) error {
	if ctx == nil {
		return wrapError("migrate", ErrorClassValidation, fmt.Errorf("context is nil"))
	}
	ordered, err := validateMigrations(migrations)
	if err != nil {
		return wrapError("migrate validate", ErrorClassMigrate, err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return wrapError("migrate table", ErrorClassMigrate, err)
	}
	current, err := currentSchemaVersion(ctx, db)
	if err != nil {
		return err
	}
	latest := latestMigrationVersion(ordered)
	if current > latest {
		return wrapError("migrate future schema", ErrorClassFuture, ErrFutureSchema)
	}
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}
	for _, migration := range ordered {
		if _, ok := applied[migration.Version]; ok {
			continue
		}
		if err := applyMigration(ctx, db, migration); err != nil {
			return err
		}
	}
	return nil
}

func validateMigrations(migrations []Migration) ([]Migration, error) {
	ordered := append([]Migration(nil), migrations...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Version < ordered[j].Version
	})
	previous := 0
	for _, migration := range ordered {
		if migration.Version <= previous {
			return nil, fmt.Errorf("migration versions must be unique and increasing")
		}
		if migration.Version != previous+1 {
			return nil, fmt.Errorf("migration version %d is not contiguous after %d", migration.Version, previous)
		}
		if migration.Name == "" {
			return nil, fmt.Errorf("migration %d name is required", migration.Version)
		}
		if len(migration.Statements) == 0 {
			return nil, fmt.Errorf("migration %d has no statements", migration.Version)
		}
		previous = migration.Version
	}
	return ordered, nil
}

func applyMigration(ctx context.Context, db *sql.DB, migration Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return wrapError("migrate begin", ErrorClassMigrate, err)
	}
	defer tx.Rollback()
	for _, statement := range migration.Statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return wrapError("migrate apply "+migration.Name, ErrorClassMigrate, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`, migration.Version, migration.Name, formatTime(nowUTC())); err != nil {
		return wrapError("migrate record "+migration.Name, ErrorClassMigrate, err)
	}
	if err := tx.Commit(); err != nil {
		return wrapError("migrate commit "+migration.Name, ErrorClassMigrate, err)
	}
	return nil
}

func currentSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, wrapError("schema version", ErrorClassQuery, err)
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

func appliedMigrations(ctx context.Context, db *sql.DB) (map[int]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, name FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, wrapError("list migrations", ErrorClassQuery, err)
	}
	defer rows.Close()
	applied := map[int]string{}
	for rows.Next() {
		var version int
		var name string
		if err := rows.Scan(&version, &name); err != nil {
			return nil, wrapError("scan migrations", ErrorClassQuery, err)
		}
		applied[version] = name
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate migrations", ErrorClassQuery, err)
	}
	return applied, nil
}

func latestMigrationVersion(migrations []Migration) int {
	latest := 0
	for _, migration := range migrations {
		if migration.Version > latest {
			latest = migration.Version
		}
	}
	return latest
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
