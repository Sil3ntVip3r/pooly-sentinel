package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

type SQLiteOptions struct {
	Path             string
	CreateParentDirs bool
	BusyTimeout      time.Duration
	WAL              bool
	Synchronous      string
}

type Store struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

func Open(ctx context.Context, opts SQLiteOptions) (*Store, error) {
	if ctx == nil {
		return nil, wrapError("open", ErrorClassValidation, fmt.Errorf("context is nil"))
	}
	if strings.TrimSpace(opts.Path) == "" {
		return nil, wrapError("open", ErrorClassValidation, fmt.Errorf("database path is required"))
	}
	if opts.BusyTimeout <= 0 {
		opts.BusyTimeout = 5 * time.Second
	}
	if opts.Synchronous == "" {
		opts.Synchronous = "NORMAL"
	}
	if !isMemoryDSN(opts.Path) {
		if !filepath.IsAbs(opts.Path) {
			return nil, wrapError("open", ErrorClassValidation, fmt.Errorf("database path must be absolute"))
		}
		if opts.CreateParentDirs {
			if err := ensureDirNoSymlink(filepath.Dir(opts.Path)); err != nil {
				return nil, wrapError("open mkdir", ErrorClassOpen, err)
			}
		}
		if err := prepareDatabaseFile(opts.Path); err != nil {
			return nil, wrapError("open database file", ErrorClassOpen, err)
		}
	}

	db, err := sql.Open(driverName, sqliteDSN(opts.Path, opts.BusyTimeout))
	if err != nil {
		return nil, wrapError("open", ErrorClassOpen, err)
	}
	store := &Store{db: db, path: opts.Path}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := initializeConnection(ctx, db, opts); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := runMigrations(ctx, db, defaultMigrations); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	if err != nil {
		return wrapError("close", ErrorClassClosed, err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	if err := db.PingContext(ctx); err != nil {
		return wrapError("ping", ErrorClassQuery, err)
	}
	return nil
}

func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	db, err := s.database()
	if err != nil {
		return 0, err
	}
	version, err := currentSchemaVersion(ctx, db)
	if err != nil {
		return 0, err
	}
	return version, nil
}

func (s *Store) WritableProbe(ctx context.Context) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return wrapError("writable probe begin", ErrorClassWrite, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS pooly_doctor_probe (id INTEGER PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return wrapError("writable probe create", ErrorClassWrite, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO pooly_doctor_probe(value) VALUES (?)`, "ok"); err != nil {
		return wrapError("writable probe insert", ErrorClassWrite, err)
	}
	return nil
}

func (s *Store) database() (*sql.DB, error) {
	if s == nil {
		return nil, wrapError("database", ErrorClassClosed, ErrClosed)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, wrapError("database", ErrorClassClosed, ErrClosed)
	}
	return s.db, nil
}

func initializeConnection(ctx context.Context, db *sql.DB, opts SQLiteOptions) error {
	if err := db.PingContext(ctx); err != nil {
		return wrapError("ping", ErrorClassOpen, err)
	}
	pragmas := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = ` + strconv.Itoa(int(opts.BusyTimeout/time.Millisecond)),
		`PRAGMA synchronous = ` + normalizeSynchronous(opts.Synchronous),
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return wrapError("pragma", ErrorClassOpen, err)
		}
	}
	if opts.WAL && !isMemoryDSN(opts.Path) {
		if _, err := db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
			return wrapError("pragma journal_mode", ErrorClassOpen, err)
		}
	}
	if err := db.PingContext(ctx); err != nil {
		return wrapError("ping after pragma", ErrorClassOpen, err)
	}
	return nil
}

func normalizeSynchronous(value string) string {
	switch strings.ToUpper(value) {
	case "OFF", "NORMAL", "FULL", "EXTRA":
		return strings.ToUpper(value)
	default:
		return "NORMAL"
	}
}

func sqliteDSN(path string, busyTimeout time.Duration) string {
	if isMemoryDSN(path) {
		return path
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("_pragma", "busy_timeout("+strconv.Itoa(int(busyTimeout/time.Millisecond))+")")
	u.RawQuery = q.Encode()
	return u.String()
}

func isMemoryDSN(path string) bool {
	return path == ":memory:" || strings.HasPrefix(path, "file::memory:") || strings.Contains(path, "mode=memory")
}

func prepareDatabaseFile(path string) error {
	file, err := openRegularNoFollow(path, os.O_CREATE|os.O_RDWR, FileMode)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !fileModeIsRestrictive(info.Mode()) {
		return fmt.Errorf("database file permissions are too permissive")
	}
	return nil
}
