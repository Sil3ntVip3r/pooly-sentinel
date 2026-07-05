package storage

import (
	"context"
	"database/sql"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

func (s *Store) UpsertCollectorState(ctx context.Context, record CollectorStateRecord) error {
	if err := validateCollectorState(record); err != nil {
		return wrapError("upsert collector state", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = nowUTC()
	}
	_, err = db.ExecContext(ctx, `INSERT INTO collector_state(
			collector, target, status, state_json, last_attempt_at, last_success_at,
			last_error_class, last_error_summary, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(collector, target) DO UPDATE SET
			status = excluded.status,
			state_json = excluded.state_json,
			last_attempt_at = excluded.last_attempt_at,
			last_success_at = excluded.last_success_at,
			last_error_class = excluded.last_error_class,
			last_error_summary = excluded.last_error_summary,
			updated_at = excluded.updated_at`,
		record.Collector,
		record.Target,
		redaction.Redact(record.Status),
		redaction.Redact(record.StateJSON),
		nullableTime(record.LastAttemptAt),
		nullableTime(record.LastSuccessAt),
		nullableString(record.LastErrorClass),
		nullableString(record.LastErrorSummary),
		formatTime(updatedAt),
	)
	if err != nil {
		return wrapError("upsert collector state", ErrorClassWrite, err)
	}
	return nil
}

func (s *Store) GetCollectorState(ctx context.Context, collector string, target string) (CollectorStateRecord, error) {
	if err := required(collector, "collector"); err != nil {
		return CollectorStateRecord{}, wrapError("get collector state", ErrorClassValidation, err)
	}
	if err := required(target, "target"); err != nil {
		return CollectorStateRecord{}, wrapError("get collector state", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return CollectorStateRecord{}, err
	}
	row := db.QueryRowContext(ctx, `SELECT collector, target, status, state_json, last_attempt_at, last_success_at,
		last_error_class, last_error_summary, updated_at
		FROM collector_state WHERE collector = ? AND target = ?`, collector, target)
	return scanCollectorState(row)
}

func (s *Store) ListCollectorState(ctx context.Context) ([]CollectorStateRecord, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT collector, target, status, state_json, last_attempt_at, last_success_at,
		last_error_class, last_error_summary, updated_at
		FROM collector_state ORDER BY collector, target`)
	if err != nil {
		return nil, wrapError("list collector state", ErrorClassQuery, err)
	}
	defer rows.Close()
	var records []CollectorStateRecord
	for rows.Next() {
		record, err := scanCollectorState(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate collector state", ErrorClassQuery, err)
	}
	return records, nil
}

type collectorStateScanner interface {
	Scan(dest ...any) error
}

func scanCollectorState(scanner collectorStateScanner) (CollectorStateRecord, error) {
	var record CollectorStateRecord
	var lastAttemptAt, lastSuccessAt sql.NullString
	var lastErrorClass, lastErrorSummary sql.NullString
	var updatedAt string
	if err := scanner.Scan(
		&record.Collector,
		&record.Target,
		&record.Status,
		&record.StateJSON,
		&lastAttemptAt,
		&lastSuccessAt,
		&lastErrorClass,
		&lastErrorSummary,
		&updatedAt,
	); err != nil {
		return CollectorStateRecord{}, classifyQueryErr("scan collector state", err)
	}
	var err error
	record.LastAttemptAt, err = scanNullableTime(lastAttemptAt)
	if err != nil {
		return CollectorStateRecord{}, wrapError("scan collector state last_attempt_at", ErrorClassQuery, err)
	}
	record.LastSuccessAt, err = scanNullableTime(lastSuccessAt)
	if err != nil {
		return CollectorStateRecord{}, wrapError("scan collector state last_success_at", ErrorClassQuery, err)
	}
	record.LastErrorClass = stringFromNull(lastErrorClass)
	record.LastErrorSummary = stringFromNull(lastErrorSummary)
	record.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return CollectorStateRecord{}, wrapError("scan collector state updated_at", ErrorClassQuery, err)
	}
	return record, nil
}
