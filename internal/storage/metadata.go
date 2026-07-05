package storage

import (
	"context"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

func (s *Store) UpsertMetadata(ctx context.Context, record MetadataRecord) error {
	if err := validateMetadata(record); err != nil {
		return wrapError("upsert metadata", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = nowUTC()
	}
	_, err = db.ExecContext(ctx, `INSERT INTO metadata(key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		record.Key, redaction.Redact(record.Value), formatTime(updatedAt))
	if err != nil {
		return wrapError("upsert metadata", ErrorClassWrite, err)
	}
	return nil
}

func (s *Store) GetMetadata(ctx context.Context, key string) (MetadataRecord, error) {
	if err := required(key, "metadata key"); err != nil {
		return MetadataRecord{}, wrapError("get metadata", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return MetadataRecord{}, err
	}
	var record MetadataRecord
	var updatedAt string
	err = db.QueryRowContext(ctx, `SELECT key, value, updated_at FROM metadata WHERE key = ?`, key).Scan(&record.Key, &record.Value, &updatedAt)
	if err != nil {
		return MetadataRecord{}, classifyQueryErr("get metadata", err)
	}
	record.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return MetadataRecord{}, wrapError("scan metadata", ErrorClassQuery, err)
	}
	return record, nil
}

func (s *Store) ListMetadata(ctx context.Context) ([]MetadataRecord, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT key, value, updated_at FROM metadata ORDER BY key`)
	if err != nil {
		return nil, wrapError("list metadata", ErrorClassQuery, err)
	}
	defer rows.Close()
	var records []MetadataRecord
	for rows.Next() {
		var record MetadataRecord
		var updatedAt string
		if err := rows.Scan(&record.Key, &record.Value, &updatedAt); err != nil {
			return nil, wrapError("scan metadata", ErrorClassQuery, err)
		}
		record.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, wrapError("scan metadata time", ErrorClassQuery, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate metadata", ErrorClassQuery, err)
	}
	return records, nil
}

func (s *Store) DeleteMetadata(ctx context.Context, key string) error {
	if err := required(key, "metadata key"); err != nil {
		return wrapError("delete metadata", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `DELETE FROM metadata WHERE key = ?`, key)
	if err != nil {
		return wrapError("delete metadata", ErrorClassWrite, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return wrapError("delete metadata rows", ErrorClassQuery, err)
	}
	if rows == 0 {
		return notFound("delete metadata")
	}
	return nil
}
