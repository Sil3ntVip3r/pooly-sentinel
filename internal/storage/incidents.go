package storage

import (
	"context"
	"database/sql"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type incidentExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type incidentQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) UpsertIncident(ctx context.Context, record IncidentRecord) error {
	if err := validateIncident(record); err != nil {
		return wrapError("upsert incident", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	return upsertIncident(ctx, db, record)
}

func upsertIncident(ctx context.Context, execer incidentExecutor, record IncidentRecord) error {
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = nowUTC()
	}
	_, err := execer.ExecContext(ctx, `INSERT INTO incidents(
			id, fingerprint, node_id, type, target, condition, severity, status, summary,
			first_seen, last_seen, last_alerted, occurrence_count, evidence_path,
			resolved_at, last_transition, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			fingerprint = excluded.fingerprint,
			node_id = excluded.node_id,
			type = excluded.type,
			target = excluded.target,
			condition = excluded.condition,
			severity = excluded.severity,
			status = excluded.status,
			summary = excluded.summary,
			first_seen = excluded.first_seen,
			last_seen = excluded.last_seen,
			last_alerted = excluded.last_alerted,
			occurrence_count = excluded.occurrence_count,
			evidence_path = excluded.evidence_path,
			resolved_at = excluded.resolved_at,
			last_transition = excluded.last_transition,
			updated_at = excluded.updated_at`,
		record.ID,
		redaction.Redact(record.Fingerprint),
		record.NodeID,
		record.Type,
		record.Target,
		record.Condition,
		redaction.Redact(record.Severity),
		redaction.Redact(record.Status),
		redaction.Redact(record.Summary),
		formatTime(record.FirstSeen),
		formatTime(record.LastSeen),
		nullableTime(record.LastAlerted),
		record.OccurrenceCount,
		nullableString(record.EvidencePath),
		nullableTime(record.ResolvedAt),
		nullableTime(record.LastTransition),
		formatTime(updatedAt),
	)
	if err != nil {
		return wrapError("upsert incident", ErrorClassWrite, err)
	}
	return nil
}

func (s *Store) GetIncident(ctx context.Context, id string) (IncidentRecord, error) {
	if err := required(id, "incident id"); err != nil {
		return IncidentRecord{}, wrapError("get incident", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return IncidentRecord{}, err
	}
	return getIncident(ctx, db, id)
}

func (s *Store) GetIncidentByFingerprint(ctx context.Context, fingerprint string) (IncidentRecord, error) {
	if err := required(fingerprint, "incident fingerprint"); err != nil {
		return IncidentRecord{}, wrapError("get incident by fingerprint", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return IncidentRecord{}, err
	}
	return getIncidentByFingerprint(ctx, db, fingerprint)
}

func getIncident(ctx context.Context, queryer incidentQueryer, id string) (IncidentRecord, error) {
	row := queryer.QueryRowContext(ctx, `SELECT id, node_id, type, target, condition, severity, status, summary,
		first_seen, last_seen, last_alerted, occurrence_count, evidence_path, resolved_at, updated_at,
		COALESCE(fingerprint, ''), last_transition
		FROM incidents WHERE id = ?`, id)
	return scanIncident(row)
}

func getIncidentByFingerprint(ctx context.Context, queryer incidentQueryer, fingerprint string) (IncidentRecord, error) {
	row := queryer.QueryRowContext(ctx, `SELECT id, node_id, type, target, condition, severity, status, summary,
		first_seen, last_seen, last_alerted, occurrence_count, evidence_path, resolved_at, updated_at,
		COALESCE(fingerprint, ''), last_transition
		FROM incidents WHERE fingerprint = ?`, fingerprint)
	return scanIncident(row)
}

func (s *Store) ListIncidents(ctx context.Context) ([]IncidentRecord, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, node_id, type, target, condition, severity, status, summary,
		first_seen, last_seen, last_alerted, occurrence_count, evidence_path, resolved_at, updated_at,
		COALESCE(fingerprint, ''), last_transition
		FROM incidents ORDER BY updated_at DESC, id`)
	if err != nil {
		return nil, wrapError("list incidents", ErrorClassQuery, err)
	}
	defer rows.Close()
	var records []IncidentRecord
	for rows.Next() {
		record, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate incidents", ErrorClassQuery, err)
	}
	return records, nil
}

type incidentScanner interface {
	Scan(dest ...any) error
}

func scanIncident(scanner incidentScanner) (IncidentRecord, error) {
	var record IncidentRecord
	var firstSeen, lastSeen, updatedAt string
	var lastAlerted, evidencePath, resolvedAt, lastTransition sql.NullString
	if err := scanner.Scan(
		&record.ID,
		&record.NodeID,
		&record.Type,
		&record.Target,
		&record.Condition,
		&record.Severity,
		&record.Status,
		&record.Summary,
		&firstSeen,
		&lastSeen,
		&lastAlerted,
		&record.OccurrenceCount,
		&evidencePath,
		&resolvedAt,
		&updatedAt,
		&record.Fingerprint,
		&lastTransition,
	); err != nil {
		return IncidentRecord{}, classifyQueryErr("scan incident", err)
	}
	var err error
	record.FirstSeen, err = parseTime(firstSeen)
	if err != nil {
		return IncidentRecord{}, wrapError("scan incident first_seen", ErrorClassQuery, err)
	}
	record.LastSeen, err = parseTime(lastSeen)
	if err != nil {
		return IncidentRecord{}, wrapError("scan incident last_seen", ErrorClassQuery, err)
	}
	record.LastAlerted, err = scanNullableTime(lastAlerted)
	if err != nil {
		return IncidentRecord{}, wrapError("scan incident last_alerted", ErrorClassQuery, err)
	}
	record.EvidencePath = stringFromNull(evidencePath)
	record.ResolvedAt, err = scanNullableTime(resolvedAt)
	if err != nil {
		return IncidentRecord{}, wrapError("scan incident resolved_at", ErrorClassQuery, err)
	}
	record.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return IncidentRecord{}, wrapError("scan incident updated_at", ErrorClassQuery, err)
	}
	record.LastTransition, err = scanNullableTime(lastTransition)
	if err != nil {
		return IncidentRecord{}, wrapError("scan incident last_transition", ErrorClassQuery, err)
	}
	return record, nil
}
