package storage

import (
	"context"
	"database/sql"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

func (s *Store) UpsertIncident(ctx context.Context, record IncidentRecord) error {
	if err := validateIncident(record); err != nil {
		return wrapError("upsert incident", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = nowUTC()
	}
	_, err = db.ExecContext(ctx, `INSERT INTO incidents(
			id, node_id, type, target, condition, severity, status, summary,
			first_seen, last_seen, last_alerted, occurrence_count, evidence_path,
			resolved_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
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
			updated_at = excluded.updated_at`,
		record.ID,
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
	row := db.QueryRowContext(ctx, `SELECT id, node_id, type, target, condition, severity, status, summary,
		first_seen, last_seen, last_alerted, occurrence_count, evidence_path, resolved_at, updated_at
		FROM incidents WHERE id = ?`, id)
	return scanIncident(row)
}

func (s *Store) ListIncidents(ctx context.Context) ([]IncidentRecord, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, node_id, type, target, condition, severity, status, summary,
		first_seen, last_seen, last_alerted, occurrence_count, evidence_path, resolved_at, updated_at
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
	var lastAlerted, evidencePath, resolvedAt sql.NullString
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
	return record, nil
}
