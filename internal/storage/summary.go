package storage

import (
	"context"
	"fmt"
)

func (s *Store) ListRecentIncidents(ctx context.Context, limit int) ([]IncidentRecord, error) {
	if limit <= 0 || limit > 1000 {
		return nil, wrapError("list recent incidents", ErrorClassValidation, fmt.Errorf("limit must be between 1 and 1000"))
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, node_id, type, target, condition, severity, status, summary,
		first_seen, last_seen, last_alerted, occurrence_count, evidence_path, resolved_at, updated_at,
		COALESCE(fingerprint, ''), last_transition
		FROM incidents ORDER BY updated_at DESC, id LIMIT ?`, limit)
	if err != nil {
		return nil, wrapError("list recent incidents", ErrorClassQuery, err)
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
		return nil, wrapError("iterate recent incidents", ErrorClassQuery, err)
	}
	return records, nil
}

func (s *Store) IncidentStatusCounts(ctx context.Context) (map[string]int64, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT status, COUNT(*) FROM incidents GROUP BY status`)
	if err != nil {
		return nil, wrapError("count incident statuses", ErrorClassQuery, err)
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, classifyQueryErr("scan incident status count", err)
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate incident status counts", ErrorClassQuery, err)
	}
	return counts, nil
}

func (s *Store) NotificationDeliveryStatusCounts(ctx context.Context) (map[string]int64, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT status, COUNT(*) FROM notification_deliveries GROUP BY status`)
	if err != nil {
		return nil, wrapError("count notification deliveries", ErrorClassQuery, err)
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, classifyQueryErr("scan notification delivery count", err)
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate notification delivery counts", ErrorClassQuery, err)
	}
	return counts, nil
}

func (s *Store) ListRecentNotificationDeliveries(ctx context.Context, incidentID string, limit int) ([]NotificationDeliveryRecord, error) {
	if limit <= 0 || limit > 1000 {
		return nil, wrapError("list recent notification deliveries", ErrorClassValidation, fmt.Errorf("limit must be between 1 and 1000"))
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	query := `SELECT id, incident_id, receiver, cost_class, status, attempt,
		attempted_at, delivered_at, error_class, error_summary
		FROM notification_deliveries ORDER BY attempted_at DESC, id LIMIT ?`
	args := []any{limit}
	if incidentID != "" {
		query = `SELECT id, incident_id, receiver, cost_class, status, attempt,
			attempted_at, delivered_at, error_class, error_summary
			FROM notification_deliveries WHERE incident_id = ? ORDER BY attempted_at DESC, id LIMIT ?`
		args = []any{incidentID, limit}
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, wrapError("list recent notification deliveries", ErrorClassQuery, err)
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
		return nil, wrapError("iterate recent notification deliveries", ErrorClassQuery, err)
	}
	return records, nil
}
