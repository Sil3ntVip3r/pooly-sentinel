package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

func required(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: redaction.Redact(value), Valid: true}
}

func stringFromNull(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func classifyQueryErr(op string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return notFound(op)
	}
	return wrapError(op, ErrorClassQuery, err)
}

func validateMetadata(record MetadataRecord) error {
	if err := required(record.Key, "metadata key"); err != nil {
		return err
	}
	return nil
}

func validateCollectorState(record CollectorStateRecord) error {
	if err := required(record.Collector, "collector"); err != nil {
		return err
	}
	if err := required(record.Target, "target"); err != nil {
		return err
	}
	if err := required(record.Status, "status"); err != nil {
		return err
	}
	return nil
}

func validateIncident(record IncidentRecord) error {
	requiredFields := map[string]string{
		"id":        record.ID,
		"node_id":   record.NodeID,
		"type":      record.Type,
		"target":    record.Target,
		"condition": record.Condition,
		"severity":  record.Severity,
		"status":    record.Status,
		"summary":   record.Summary,
	}
	for field, value := range requiredFields {
		if err := required(value, field); err != nil {
			return err
		}
	}
	if err := requiredTime("first_seen", record.FirstSeen); err != nil {
		return err
	}
	if err := requiredTime("last_seen", record.LastSeen); err != nil {
		return err
	}
	if record.OccurrenceCount < 0 {
		return fmt.Errorf("occurrence_count cannot be negative")
	}
	return nil
}

func validateNotificationDelivery(record NotificationDeliveryRecord) error {
	requiredFields := map[string]string{
		"id":          record.ID,
		"incident_id": record.IncidentID,
		"receiver":    record.Receiver,
		"cost_class":  record.CostClass,
		"status":      record.Status,
	}
	for field, value := range requiredFields {
		if err := required(value, field); err != nil {
			return err
		}
	}
	if record.Attempt < 1 {
		return fmt.Errorf("attempt must be greater than zero")
	}
	if err := requiredTime("attempted_at", record.AttemptedAt); err != nil {
		return err
	}
	return nil
}
