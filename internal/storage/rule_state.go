package storage

import (
	"context"
	"database/sql"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type ruleStateExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type ruleStateQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *Store) UpsertRuleEvaluationState(ctx context.Context, record RuleEvaluationStateRecord) error {
	if err := validateRuleEvaluationState(record); err != nil {
		return wrapError("upsert rule evaluation state", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	return upsertRuleEvaluationState(ctx, db, record)
}

func upsertRuleEvaluationState(ctx context.Context, execer ruleStateExecutor, record RuleEvaluationStateRecord) error {
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = nowUTC()
	}
	_, err := execer.ExecContext(ctx, `INSERT INTO rule_evaluation_state(
			rule_id, target, state, severity, condition_met_since, recovery_since,
			last_evaluated_at, last_observed_at, last_result_summary, pending_severity,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(rule_id, target) DO UPDATE SET
			state = excluded.state,
			severity = excluded.severity,
			condition_met_since = excluded.condition_met_since,
			recovery_since = excluded.recovery_since,
			last_evaluated_at = excluded.last_evaluated_at,
			last_observed_at = excluded.last_observed_at,
			last_result_summary = excluded.last_result_summary,
			pending_severity = excluded.pending_severity,
			updated_at = excluded.updated_at`,
		redaction.Redact(record.RuleID),
		redaction.Redact(record.Target),
		redaction.Redact(record.State),
		redaction.Redact(record.Severity),
		nullableTime(record.ConditionMetSince),
		nullableTime(record.RecoverySince),
		formatTime(record.LastEvaluatedAt),
		nullableTime(record.LastObservedAt),
		redaction.Redact(record.LastResultSummary),
		nullableString(record.PendingSeverity),
		formatTime(updatedAt),
	)
	if err != nil {
		return wrapError("upsert rule evaluation state", ErrorClassWrite, err)
	}
	return nil
}

func (s *Store) GetRuleEvaluationState(ctx context.Context, ruleID string, target string) (RuleEvaluationStateRecord, error) {
	if err := required(ruleID, "rule id"); err != nil {
		return RuleEvaluationStateRecord{}, wrapError("get rule evaluation state", ErrorClassValidation, err)
	}
	if err := required(target, "rule target"); err != nil {
		return RuleEvaluationStateRecord{}, wrapError("get rule evaluation state", ErrorClassValidation, err)
	}
	db, err := s.database()
	if err != nil {
		return RuleEvaluationStateRecord{}, err
	}
	return getRuleEvaluationState(ctx, db, ruleID, target)
}

func (s *Store) ListRuleEvaluationState(ctx context.Context) ([]RuleEvaluationStateRecord, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT rule_id, target, state, severity, condition_met_since, recovery_since,
		last_evaluated_at, last_observed_at, last_result_summary, pending_severity, updated_at
		FROM rule_evaluation_state ORDER BY rule_id, target`)
	if err != nil {
		return nil, wrapError("list rule evaluation state", ErrorClassQuery, err)
	}
	defer rows.Close()
	var records []RuleEvaluationStateRecord
	for rows.Next() {
		record, err := scanRuleEvaluationState(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError("iterate rule evaluation state", ErrorClassQuery, err)
	}
	return records, nil
}

func getRuleEvaluationState(ctx context.Context, queryer ruleStateQueryer, ruleID string, target string) (RuleEvaluationStateRecord, error) {
	row := queryer.QueryRowContext(ctx, `SELECT rule_id, target, state, severity, condition_met_since, recovery_since,
		last_evaluated_at, last_observed_at, last_result_summary, pending_severity, updated_at
		FROM rule_evaluation_state WHERE rule_id = ? AND target = ?`, ruleID, target)
	return scanRuleEvaluationState(row)
}

type ruleEvaluationStateScanner interface {
	Scan(dest ...any) error
}

func scanRuleEvaluationState(scanner ruleEvaluationStateScanner) (RuleEvaluationStateRecord, error) {
	var record RuleEvaluationStateRecord
	var conditionMetSince, recoverySince, lastObservedAt, pendingSeverity sql.NullString
	var lastEvaluatedAt, updatedAt string
	if err := scanner.Scan(
		&record.RuleID,
		&record.Target,
		&record.State,
		&record.Severity,
		&conditionMetSince,
		&recoverySince,
		&lastEvaluatedAt,
		&lastObservedAt,
		&record.LastResultSummary,
		&pendingSeverity,
		&updatedAt,
	); err != nil {
		return RuleEvaluationStateRecord{}, classifyQueryErr("scan rule evaluation state", err)
	}
	var err error
	record.ConditionMetSince, err = scanNullableTime(conditionMetSince)
	if err != nil {
		return RuleEvaluationStateRecord{}, wrapError("scan rule evaluation state condition_met_since", ErrorClassQuery, err)
	}
	record.RecoverySince, err = scanNullableTime(recoverySince)
	if err != nil {
		return RuleEvaluationStateRecord{}, wrapError("scan rule evaluation state recovery_since", ErrorClassQuery, err)
	}
	record.LastEvaluatedAt, err = parseTime(lastEvaluatedAt)
	if err != nil {
		return RuleEvaluationStateRecord{}, wrapError("scan rule evaluation state last_evaluated_at", ErrorClassQuery, err)
	}
	record.LastObservedAt, err = scanNullableTime(lastObservedAt)
	if err != nil {
		return RuleEvaluationStateRecord{}, wrapError("scan rule evaluation state last_observed_at", ErrorClassQuery, err)
	}
	record.PendingSeverity = stringFromNull(pendingSeverity)
	record.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return RuleEvaluationStateRecord{}, wrapError("scan rule evaluation state updated_at", ErrorClassQuery, err)
	}
	return record, nil
}
