package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type notificationDeliveryExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) InsertNotificationDelivery(ctx context.Context, record NotificationDeliveryRecord) error {
	if err := validateNotificationDelivery(record); err != nil {
		return wrapError("insert notification delivery", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	return insertNotificationDelivery(ctx, db, record)
}

func insertNotificationDelivery(ctx context.Context, execer notificationDeliveryExecutor, record NotificationDeliveryRecord) error {
	_, err := execer.ExecContext(ctx, `INSERT INTO notification_deliveries(
			id, incident_id, receiver, cost_class, status, attempt, attempted_at,
			delivered_at, error_class, error_summary
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.IncidentID,
		record.Receiver,
		record.CostClass,
		redaction.Redact(record.Status),
		record.Attempt,
		formatTime(record.AttemptedAt),
		nullableTime(record.DeliveredAt),
		nullableString(record.ErrorClass),
		nullableString(record.ErrorSummary),
	)
	if err != nil {
		return wrapError("insert notification delivery", ErrorClassWrite, err)
	}
	return nil
}

func (s *Store) UpdateIncidentLastAlerted(ctx context.Context, incidentID string, alertedAt time.Time) error {
	if err := required(incidentID, "incident id"); err != nil {
		return wrapError("update incident last_alerted", ErrorClassValidation, err)
	}
	if err := requiredTime("last_alerted", alertedAt); err != nil {
		return wrapError("update incident last_alerted", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	return updateIncidentLastAlerted(ctx, db, incidentID, alertedAt)
}

func updateIncidentLastAlerted(ctx context.Context, execer notificationDeliveryExecutor, incidentID string, alertedAt time.Time) error {
	result, err := execer.ExecContext(ctx, `UPDATE incidents SET last_alerted = ?, updated_at = ? WHERE id = ?`,
		formatTime(alertedAt), formatTime(nowUTC()), incidentID)
	if err != nil {
		return wrapError("update incident last_alerted", ErrorClassWrite, err)
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		return notFound("update incident last_alerted")
	}
	return nil
}

func (s *Store) ListNotificationDeliveries(ctx context.Context, incidentID string) ([]NotificationDeliveryRecord, error) {
	if err := required(incidentID, "incident id"); err != nil {
		return nil, wrapError("list notification deliveries", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, incident_id, receiver, cost_class, status, attempt,
		attempted_at, delivered_at, error_class, error_summary
		FROM notification_deliveries WHERE incident_id = ? ORDER BY attempted_at, id`, incidentID)
	if err != nil {
		return nil, wrapError("list notification deliveries", ErrorClassQuery, err)
	}
	defer rows.Close()
	var records []NotificationDeliveryRecord
	for rows.Next() {
		record, err := scanNotificationDelivery(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate notification deliveries", ErrorClassQuery, err)
	}
	return records, nil
}

type notificationDeliveryScanner interface {
	Scan(dest ...any) error
}

func scanNotificationDelivery(scanner notificationDeliveryScanner) (NotificationDeliveryRecord, error) {
	var record NotificationDeliveryRecord
	var attemptedAt string
	var deliveredAt, errorClass, errorSummary sql.NullString
	if err := scanner.Scan(
		&record.ID,
		&record.IncidentID,
		&record.Receiver,
		&record.CostClass,
		&record.Status,
		&record.Attempt,
		&attemptedAt,
		&deliveredAt,
		&errorClass,
		&errorSummary,
	); err != nil {
		return NotificationDeliveryRecord{}, classifyQueryErr("scan notification delivery", err)
	}
	var err error
	record.AttemptedAt, err = parseTime(attemptedAt)
	if err != nil {
		return NotificationDeliveryRecord{}, wrapError("scan notification delivery attempted_at", ErrorClassQuery, err)
	}
	record.DeliveredAt, err = scanNullableTime(deliveredAt)
	if err != nil {
		return NotificationDeliveryRecord{}, wrapError("scan notification delivery delivered_at", ErrorClassQuery, err)
	}
	record.ErrorClass = stringFromNull(errorClass)
	record.ErrorSummary = stringFromNull(errorSummary)
	return record, nil
}
