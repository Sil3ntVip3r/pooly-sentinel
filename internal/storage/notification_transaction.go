package storage

import (
	"context"
	"database/sql"
	"time"
)

type NotificationDeliveryTransaction interface {
	InsertNotificationDelivery(ctx context.Context, record NotificationDeliveryRecord) error
	UpdateIncidentLastAlerted(ctx context.Context, incidentID string, alertedAt time.Time) error
}

func (s *Store) NotificationDeliveryTransaction(ctx context.Context, fn func(NotificationDeliveryTransaction) error) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return wrapError("notification delivery transaction begin", ErrorClassWrite, err)
	}
	defer tx.Rollback()
	if err := fn(notificationDeliveryTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return wrapError("notification delivery transaction commit", ErrorClassWrite, err)
	}
	return nil
}

type notificationDeliveryTx struct {
	tx *sql.Tx
}

func (t notificationDeliveryTx) InsertNotificationDelivery(ctx context.Context, record NotificationDeliveryRecord) error {
	if err := validateNotificationDelivery(record); err != nil {
		return wrapError("insert notification delivery", ErrorClassValidation, err)
	}
	return insertNotificationDelivery(ctx, t.tx, record)
}

func (t notificationDeliveryTx) UpdateIncidentLastAlerted(ctx context.Context, incidentID string, alertedAt time.Time) error {
	if err := required(incidentID, "incident id"); err != nil {
		return wrapError("update incident last_alerted", ErrorClassValidation, err)
	}
	if err := requiredTime("last_alerted", alertedAt); err != nil {
		return wrapError("update incident last_alerted", ErrorClassValidation, err)
	}
	return updateIncidentLastAlerted(ctx, t.tx, incidentID, alertedAt)
}
