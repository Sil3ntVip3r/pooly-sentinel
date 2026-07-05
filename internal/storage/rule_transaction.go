package storage

import (
	"context"
	"database/sql"
)

type RuleEvaluationTransaction interface {
	GetRuleEvaluationState(ctx context.Context, ruleID string, target string) (RuleEvaluationStateRecord, error)
	UpsertRuleEvaluationState(ctx context.Context, record RuleEvaluationStateRecord) error
	GetIncidentByFingerprint(ctx context.Context, fingerprint string) (IncidentRecord, error)
	UpsertIncident(ctx context.Context, record IncidentRecord) error
}

func (s *Store) RuleEvaluationTransaction(ctx context.Context, fn func(RuleEvaluationTransaction) error) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return wrapError("rule evaluation transaction begin", ErrorClassWrite, err)
	}
	defer tx.Rollback()
	if err := fn(ruleEvaluationTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return wrapError("rule evaluation transaction commit", ErrorClassWrite, err)
	}
	return nil
}

type ruleEvaluationTx struct {
	tx *sql.Tx
}

func (t ruleEvaluationTx) GetRuleEvaluationState(ctx context.Context, ruleID string, target string) (RuleEvaluationStateRecord, error) {
	if err := required(ruleID, "rule id"); err != nil {
		return RuleEvaluationStateRecord{}, wrapError("get rule evaluation state", ErrorClassValidation, err)
	}
	if err := required(target, "rule target"); err != nil {
		return RuleEvaluationStateRecord{}, wrapError("get rule evaluation state", ErrorClassValidation, err)
	}
	return getRuleEvaluationState(ctx, t.tx, ruleID, target)
}

func (t ruleEvaluationTx) UpsertRuleEvaluationState(ctx context.Context, record RuleEvaluationStateRecord) error {
	if err := validateRuleEvaluationState(record); err != nil {
		return wrapError("upsert rule evaluation state", ErrorClassValidation, err)
	}
	return upsertRuleEvaluationState(ctx, t.tx, record)
}

func (t ruleEvaluationTx) GetIncidentByFingerprint(ctx context.Context, fingerprint string) (IncidentRecord, error) {
	if err := required(fingerprint, "incident fingerprint"); err != nil {
		return IncidentRecord{}, wrapError("get incident by fingerprint", ErrorClassValidation, err)
	}
	return getIncidentByFingerprint(ctx, t.tx, fingerprint)
}

func (t ruleEvaluationTx) UpsertIncident(ctx context.Context, record IncidentRecord) error {
	if err := validateIncident(record); err != nil {
		return wrapError("upsert incident", ErrorClassValidation, err)
	}
	return upsertIncident(ctx, t.tx, record)
}
