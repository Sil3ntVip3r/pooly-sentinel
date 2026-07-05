package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type DoctorOptions struct {
	StateDir           string
	LogDir             string
	DatabaseFile       string
	CurrentMetricsFile string
	BusyTimeout        time.Duration
	WAL                bool
}

type DoctorStatus string

const (
	DoctorPass DoctorStatus = "PASS"
	DoctorWarn DoctorStatus = "WARN"
	DoctorFail DoctorStatus = "FAIL"
)

type DoctorCheck struct {
	Name    string
	Status  DoctorStatus
	Message string
}

func RunDoctor(ctx context.Context, opts DoctorOptions) []DoctorCheck {
	var checks []DoctorCheck
	add := func(name string, status DoctorStatus, message string) {
		checks = append(checks, DoctorCheck{Name: name, Status: status, Message: message})
	}

	if ctx == nil {
		add("context", DoctorFail, "context is nil")
		return checks
	}
	if opts.DatabaseFile == "" {
		opts.DatabaseFile = DefaultDatabaseFile
	}
	if opts.CurrentMetricsFile == "" {
		opts.CurrentMetricsFile = DefaultCurrentMetricsFile
	}
	if opts.BusyTimeout <= 0 {
		opts.BusyTimeout = 5 * time.Second
	}

	if err := validateDoctorOptions(opts); err != nil {
		add("storage configuration", DoctorFail, err.Error())
		return checks
	}
	add("storage configuration", DoctorPass, "storage paths are configured")

	for _, dir := range []struct {
		name string
		path string
	}{
		{name: "state directory", path: opts.StateDir},
		{name: "log directory", path: opts.LogDir},
	} {
		if err := ensureDir(dir.path); err != nil {
			add(dir.name, DoctorFail, err.Error())
			return checks
		}
		add(dir.name, DoctorPass, "directory exists or was created")
		if err := probeWritableFile(ctx, dir.path); err != nil {
			add(dir.name+" writable probe", DoctorFail, err.Error())
			return checks
		}
		add(dir.name+" writable probe", DoctorPass, "temporary probe succeeded")
	}

	dbPath := filepath.Join(opts.StateDir, opts.DatabaseFile)
	store, err := Open(ctx, SQLiteOptions{
		Path:             dbPath,
		CreateParentDirs: true,
		BusyTimeout:      opts.BusyTimeout,
		WAL:              opts.WAL,
		Synchronous:      "NORMAL",
	})
	if err != nil {
		add("database open", DoctorFail, err.Error())
		return checks
	}
	defer store.Close()
	add("database open", DoctorPass, "database opened")

	if err := store.Ping(ctx); err != nil {
		add("database ping", DoctorFail, err.Error())
		return checks
	}
	add("database ping", DoctorPass, "ping succeeded")

	version, err := store.SchemaVersion(ctx)
	if err != nil {
		add("migration version", DoctorFail, err.Error())
		return checks
	}
	add("migration version", DoctorPass, fmt.Sprintf("schema version %d", version))

	if err := store.WritableProbe(ctx); err != nil {
		add("writable transaction", DoctorFail, err.Error())
		return checks
	}
	add("writable transaction", DoctorPass, "temporary transaction succeeded")

	currentProbe := filepath.Join(opts.StateDir, ".pooly-doctor-current-state.json")
	_ = os.Remove(currentProbe)
	if err := WriteCurrentState(ctx, currentProbe, map[string]any{"doctor": "ok"}); err != nil {
		add("current-state atomic write", DoctorFail, err.Error())
		return checks
	}
	_ = os.Remove(currentProbe)
	add("current-state atomic write", DoctorPass, "temporary current-state write succeeded")

	eventPath := filepath.Join(opts.LogDir, "events", ".pooly-doctor.jsonl")
	_ = os.Remove(eventPath)
	writer, err := OpenEventWriter(ctx, EventWriterOptions{Path: eventPath, MaxEventBytes: 4096, SyncOnWrite: true})
	if err != nil {
		add("jsonl append", DoctorFail, err.Error())
		return checks
	}
	if err := writer.Write(ctx, map[string]any{"doctor": "ok"}); err != nil {
		_ = writer.Close()
		add("jsonl append", DoctorFail, err.Error())
		return checks
	}
	if err := writer.Close(); err != nil {
		add("jsonl append", DoctorFail, err.Error())
		return checks
	}
	_ = os.Remove(eventPath)
	add("jsonl append", DoctorPass, "temporary event append succeeded")

	return checks
}

func DoctorFailed(checks []DoctorCheck) bool {
	for _, check := range checks {
		if check.Status == DoctorFail {
			return true
		}
	}
	return false
}

func validateDoctorOptions(opts DoctorOptions) error {
	if opts.StateDir == "" {
		return fmt.Errorf("state_dir is required")
	}
	if opts.LogDir == "" {
		return fmt.Errorf("log_dir is required")
	}
	if !filepath.IsAbs(opts.StateDir) || !filepath.IsAbs(opts.LogDir) {
		return fmt.Errorf("storage directories must be absolute paths")
	}
	if err := validatePlainFilename(opts.DatabaseFile); err != nil {
		return fmt.Errorf("database_file: %w", err)
	}
	if err := validatePlainFilename(opts.CurrentMetricsFile); err != nil {
		return fmt.Errorf("current_metrics_file: %w", err)
	}
	return nil
}

func probeWritableFile(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".pooly-doctor-probe-*")
	if err != nil {
		return err
	}
	path := file.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.WriteString("ok\n"); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
