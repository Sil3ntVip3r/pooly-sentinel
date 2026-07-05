package rules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/incidents"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type Store interface {
	GetRuleEvaluationState(ctx context.Context, ruleID string, target string) (storage.RuleEvaluationStateRecord, error)
	UpsertRuleEvaluationState(ctx context.Context, record storage.RuleEvaluationStateRecord) error
	incidents.Store
}

type Engine struct {
	Rules  []Rule
	NodeID string
	Clock  Clock
}

type Result struct {
	RuleID             string                `json:"rule_id"`
	Target             string                `json:"target"`
	State              State                 `json:"state"`
	Severity           Severity              `json:"severity"`
	Matched            bool                  `json:"matched"`
	Summary            string                `json:"summary"`
	IncidentTransition *incidents.Transition `json:"incident_transition,omitempty"`
	Labels             map[string]string     `json:"labels,omitempty"`
}

type Evaluation struct {
	Results     []Result               `json:"results"`
	Transitions []incidents.Transition `json:"transitions"`
}

func (e Engine) Evaluate(ctx context.Context, store Store, observations []resources.Observation) (Evaluation, error) {
	if ctx == nil {
		return Evaluation{}, fmt.Errorf("context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Evaluation{}, err
	}
	if store == nil {
		return Evaluation{}, fmt.Errorf("rule store is nil")
	}
	clock := e.Clock
	if clock == nil {
		clock = RealClock{}
	}
	now := clock.Now().UTC()
	incidentEngine := incidents.NewEngine(func() time.Time { return now })
	var evaluation Evaluation
	for _, rule := range e.Rules {
		if err := ctx.Err(); err != nil {
			return evaluation, err
		}
		if !rule.Enabled {
			continue
		}
		targets := rule.matchingTargets(observations)
		for _, target := range targets {
			result, transition, err := e.evaluateMaybeTransactional(ctx, store, incidentEngine, rule, target, observations, now)
			if err != nil {
				return evaluation, err
			}
			evaluation.Results = append(evaluation.Results, result)
			if transition != nil && transition.Action != incidents.ActionNone {
				evaluation.Transitions = append(evaluation.Transitions, *transition)
			}
		}
	}
	return evaluation, nil
}

type transactionalStore interface {
	RuleEvaluationTransaction(ctx context.Context, fn func(storage.RuleEvaluationTransaction) error) error
}

func (e Engine) evaluateMaybeTransactional(ctx context.Context, store Store, incidentEngine incidents.Engine, rule Rule, target string, observations []resources.Observation, now time.Time) (Result, *incidents.Transition, error) {
	transactional, ok := store.(transactionalStore)
	if !ok {
		return e.evaluateRuleTarget(ctx, store, incidentEngine, rule, target, observations, now)
	}
	var result Result
	var transition *incidents.Transition
	err := transactional.RuleEvaluationTransaction(ctx, func(tx storage.RuleEvaluationTransaction) error {
		var err error
		result, transition, err = e.evaluateRuleTarget(ctx, tx, incidentEngine, rule, target, observations, now)
		return err
	})
	if err != nil {
		return Result{}, nil, err
	}
	return result, transition, nil
}

func (e Engine) evaluateRuleTarget(ctx context.Context, store Store, incidentEngine incidents.Engine, rule Rule, target string, observations []resources.Observation, now time.Time) (Result, *incidents.Transition, error) {
	previous, err := loadMemory(ctx, store, rule.ID, target)
	if err != nil {
		return Result{}, nil, err
	}
	status := observationStatus(rule, observations)
	if status.class != "" {
		return e.evaluatePolicyStatus(ctx, store, incidentEngine, rule, target, previous, status, now)
	}
	desired, reason, observedAt, matched := rule.desiredSeverityForTarget(observations, target)
	if !matched {
		return e.evaluateMissing(ctx, store, incidentEngine, rule, target, previous, now)
	}
	memory, candidate, result := transitionState(rule, target, previous, desired, reason, observedAt, now)
	result.RuleID = rule.ID
	result.Target = target
	result.Matched = true
	result.Labels = safeLabels(rule.Labels)
	if err := saveMemory(ctx, store, rule.ID, target, memory); err != nil {
		return Result{}, nil, err
	}
	var transition *incidents.Transition
	if candidate != nil {
		candidate.NodeID = e.NodeID
		got, err := incidentEngine.Apply(ctx, store, *candidate)
		if err != nil {
			return Result{}, nil, err
		}
		transition = &got
		result.IncidentTransition = transition
	}
	return result, transition, nil
}

func (e Engine) evaluateMissing(ctx context.Context, store Store, incidentEngine incidents.Engine, rule Rule, target string, previous EvaluationMemory, now time.Time) (Result, *incidents.Transition, error) {
	policy := rule.MissingData
	if policy == "" {
		policy = PolicyStale
	}
	if policy == PolicyWarn || policy == PolicyFail {
		desired := SeverityWarning
		if policy == PolicyFail {
			desired = SeverityFailure
		}
		memory, candidate, result := transitionState(rule, target, previous, desired, "missing data policy matched", now, now)
		result.RuleID = rule.ID
		result.Target = target
		result.Matched = false
		result.Labels = safeLabels(rule.Labels)
		if err := saveMemory(ctx, store, rule.ID, target, memory); err != nil {
			return Result{}, nil, err
		}
		var transition *incidents.Transition
		if candidate != nil {
			candidate.NodeID = e.NodeID
			got, err := incidentEngine.Apply(ctx, store, *candidate)
			if err != nil {
				return Result{}, nil, err
			}
			transition = &got
			result.IncidentTransition = transition
		}
		return result, transition, nil
	}
	state := StateStale
	if policy == PolicyIgnore {
		state = previous.State
		if state == "" {
			state = StateOK
		}
	}
	memory := previous
	memory.LastEvaluatedAt = now
	memory.LastResultSummary = "missing data"
	if err := saveMemory(ctx, store, rule.ID, target, memory); err != nil {
		return Result{}, nil, err
	}
	return Result{RuleID: rule.ID, Target: target, State: state, Severity: memory.Severity, Matched: false, Summary: "missing data", Labels: safeLabels(rule.Labels)}, nil, nil
}

func (e Engine) evaluatePolicyStatus(ctx context.Context, store Store, incidentEngine incidents.Engine, rule Rule, target string, previous EvaluationMemory, status ruleObservationStatus, now time.Time) (Result, *incidents.Transition, error) {
	if status.class == resources.ErrorUnsupported || status.class == resources.ErrorCounterReset {
		memory := previous
		if memory.State == "" {
			memory.State = StateOK
			memory.Severity = SeverityNone
		}
		memory.LastEvaluatedAt = now
		memory.LastResultSummary = status.summary
		if err := saveMemory(ctx, store, rule.ID, target, memory); err != nil {
			return Result{}, nil, err
		}
		state := StateUnknown
		if status.class == resources.ErrorCounterReset {
			state = StateStale
		}
		return Result{RuleID: rule.ID, Target: target, State: state, Severity: memory.Severity, Matched: false, Summary: status.summary, Labels: safeLabels(rule.Labels)}, nil, nil
	}
	policy := rule.StaleData
	if policy == "" {
		policy = PolicyStale
	}
	if policy == PolicyWarn || policy == PolicyFail {
		desired := SeverityWarning
		if policy == PolicyFail {
			desired = SeverityFailure
		}
		memory, candidate, result := transitionState(rule, target, previous, desired, status.summary, now, now)
		result.RuleID = rule.ID
		result.Target = target
		result.Matched = false
		result.Labels = safeLabels(rule.Labels)
		if err := saveMemory(ctx, store, rule.ID, target, memory); err != nil {
			return Result{}, nil, err
		}
		var transition *incidents.Transition
		if candidate != nil {
			candidate.NodeID = e.NodeID
			got, err := incidentEngine.Apply(ctx, store, *candidate)
			if err != nil {
				return Result{}, nil, err
			}
			transition = &got
			result.IncidentTransition = transition
		}
		return result, transition, nil
	}
	memory := previous
	if memory.State == "" {
		memory.State = StateOK
		memory.Severity = SeverityNone
	}
	memory.LastEvaluatedAt = now
	memory.LastResultSummary = status.summary
	if err := saveMemory(ctx, store, rule.ID, target, memory); err != nil {
		return Result{}, nil, err
	}
	return Result{RuleID: rule.ID, Target: target, State: StateStale, Severity: memory.Severity, Matched: false, Summary: status.summary, Labels: safeLabels(rule.Labels)}, nil, nil
}

func transitionState(rule Rule, target string, previous EvaluationMemory, desired Severity, reason string, observedAt time.Time, now time.Time) (EvaluationMemory, *incidents.Candidate, Result) {
	memory := previous
	if memory.State == "" {
		memory.State = StateOK
		memory.Severity = SeverityNone
	}
	memory.LastEvaluatedAt = now
	if !observedAt.IsZero() {
		observed := observedAt.UTC()
		memory.LastObservedAt = &observed
	}
	memory.LastResultSummary = safeSummary(renderSummary(rule, target, desired, reason))

	if desired == SeverityNone {
		if isActive(memory.State) {
			if memory.RecoverySince == nil {
				since := now
				memory.RecoverySince = &since
				memory.State = StateRecovering
				return memory, nil, Result{State: StateRecovering, Severity: memory.Severity, Summary: memory.LastResultSummary}
			}
			if now.Sub(*memory.RecoverySince) >= rule.RecoverFor {
				memory.State = StateRecovered
				memory.Severity = SeverityNone
				memory.ConditionMetSince = nil
				memory.PendingSeverity = SeverityNone
				candidate := incidentCandidate(rule, target, SeverityNone, false, memory.LastResultSummary, now)
				return memory, &candidate, Result{State: StateRecovered, Severity: SeverityNone, Summary: memory.LastResultSummary}
			}
			memory.State = StateRecovering
			return memory, nil, Result{State: StateRecovering, Severity: memory.Severity, Summary: memory.LastResultSummary}
		}
		if memory.State == StateRecovered {
			memory.State = StateOK
		} else if !isActive(memory.State) {
			memory.State = StateOK
		}
		memory.Severity = SeverityNone
		memory.ConditionMetSince = nil
		memory.RecoverySince = nil
		memory.PendingSeverity = SeverityNone
		return memory, nil, Result{State: memory.State, Severity: SeverityNone, Summary: memory.LastResultSummary}
	}

	memory.RecoverySince = nil
	thresholdDuration := rule.durationFor(desired)
	if isActiveSeverity(memory.Severity, desired) && isActive(memory.State) {
		if desired == SeverityCritical && memory.Severity != SeverityCritical {
			memory.Severity = SeverityCritical
			memory.State = StateCritical
			candidate := incidentCandidate(rule, target, desired, true, memory.LastResultSummary, now)
			return memory, &candidate, Result{State: memory.State, Severity: memory.Severity, Summary: memory.LastResultSummary}
		}
		memory.Severity = desired
		memory.State = stateForSeverity(desired)
		candidate := incidentCandidate(rule, target, desired, true, memory.LastResultSummary, now)
		return memory, &candidate, Result{State: memory.State, Severity: memory.Severity, Summary: memory.LastResultSummary}
	}
	if memory.PendingSeverity != desired || memory.ConditionMetSince == nil {
		since := now
		memory.ConditionMetSince = &since
		memory.PendingSeverity = desired
		memory.State = pendingStateForSeverity(desired)
		return memory, nil, Result{State: memory.State, Severity: memory.Severity, Summary: memory.LastResultSummary}
	}
	if now.Sub(*memory.ConditionMetSince) >= thresholdDuration {
		memory.Severity = desired
		memory.State = stateForSeverity(desired)
		candidate := incidentCandidate(rule, target, desired, true, memory.LastResultSummary, now)
		return memory, &candidate, Result{State: memory.State, Severity: memory.Severity, Summary: memory.LastResultSummary}
	}
	memory.State = pendingStateForSeverity(desired)
	return memory, nil, Result{State: memory.State, Severity: memory.Severity, Summary: memory.LastResultSummary}
}

func (r Rule) durationFor(severity Severity) time.Duration {
	switch severity {
	case SeverityCritical:
		if r.Critical != nil {
			return r.Critical.For
		}
	case SeverityFailure:
		if r.Fail != nil {
			return r.Fail.For
		}
	case SeverityWarning:
		if r.Warn != nil {
			return r.Warn.For
		}
	}
	return 0
}

func incidentCandidate(rule Rule, target string, severity Severity, active bool, summary string, now time.Time) incidents.Candidate {
	incidentType := rule.Collector
	if incidentType == "" {
		incidentType = "rule"
	}
	condition := rule.ID
	if rule.Metric != "" {
		condition = rule.Metric
	}
	if rule.EventCategory != "" {
		condition = rule.EventCategory
	}
	return incidents.Candidate{
		Type:       incidentType,
		Target:     target,
		Condition:  condition,
		Severity:   severity,
		Active:     active,
		Summary:    summary,
		ObservedAt: now,
	}
}

func isActive(state State) bool {
	return state == StateWarn || state == StateFail || state == StateCritical || state == StateRecovering
}

func isActiveSeverity(current Severity, desired Severity) bool {
	return severityRank(current) >= severityRank(desired)
}

func stateForSeverity(severity Severity) State {
	switch severity {
	case SeverityCritical:
		return StateCritical
	case SeverityFailure:
		return StateFail
	case SeverityWarning:
		return StateWarn
	default:
		return StateOK
	}
}

func pendingStateForSeverity(severity Severity) State {
	if severityRank(severity) >= severityRank(SeverityFailure) {
		return StatePendingFail
	}
	return StatePendingWarn
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 3
	case SeverityFailure:
		return 2
	case SeverityWarning:
		return 1
	default:
		return 0
	}
}

func loadMemory(ctx context.Context, store Store, ruleID string, target string) (EvaluationMemory, error) {
	record, err := store.GetRuleEvaluationState(ctx, ruleID, target)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return EvaluationMemory{State: StateOK, Severity: SeverityNone}, nil
		}
		return EvaluationMemory{}, err
	}
	return EvaluationMemory{
		State:             State(record.State),
		Severity:          Severity(record.Severity),
		ConditionMetSince: record.ConditionMetSince,
		RecoverySince:     record.RecoverySince,
		LastEvaluatedAt:   record.LastEvaluatedAt,
		LastObservedAt:    record.LastObservedAt,
		LastResultSummary: record.LastResultSummary,
		PendingSeverity:   Severity(record.PendingSeverity),
	}, nil
}

func saveMemory(ctx context.Context, store Store, ruleID string, target string, memory EvaluationMemory) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if memory.State == "" {
		memory.State = StateOK
	}
	if memory.Severity == "" {
		memory.Severity = SeverityNone
	}
	return store.UpsertRuleEvaluationState(ctx, storage.RuleEvaluationStateRecord{
		RuleID:            ruleID,
		Target:            target,
		State:             string(memory.State),
		Severity:          string(memory.Severity),
		ConditionMetSince: memory.ConditionMetSince,
		RecoverySince:     memory.RecoverySince,
		LastEvaluatedAt:   memory.LastEvaluatedAt,
		LastObservedAt:    memory.LastObservedAt,
		LastResultSummary: memory.LastResultSummary,
		PendingSeverity:   string(memory.PendingSeverity),
		UpdatedAt:         memory.LastEvaluatedAt,
	})
}

type ruleObservationStatus struct {
	class   resources.ErrorClass
	summary string
}

func observationStatus(rule Rule, observations []resources.Observation) ruleObservationStatus {
	for _, observation := range observations {
		if !collectorMatches(rule.Collector, observation.Collector) {
			continue
		}
		if !observation.Supported {
			return ruleObservationStatus{class: resources.ErrorUnsupported, summary: "collector unsupported"}
		}
		if observation.ErrorClass == resources.ErrorCounterReset {
			return ruleObservationStatus{class: resources.ErrorCounterReset, summary: "counter reset or first baseline"}
		}
		if observation.Stale {
			return ruleObservationStatus{class: observation.ErrorClass, summary: "stale observation"}
		}
		if !observation.Success {
			class := observation.ErrorClass
			if class == "" {
				class = resources.ErrorInternal
			}
			return ruleObservationStatus{class: class, summary: "collector failure: " + string(class)}
		}
	}
	return ruleObservationStatus{}
}

func renderSummary(rule Rule, target string, severity Severity, reason string) string {
	if rule.Summary != "" {
		summary := strings.ReplaceAll(rule.Summary, "{{rule_id}}", rule.ID)
		summary = strings.ReplaceAll(summary, "{{target}}", target)
		summary = strings.ReplaceAll(summary, "{{severity}}", string(severity))
		return summary
	}
	if severity == SeverityNone {
		return fmt.Sprintf("%s recovered for %s", rule.ID, target)
	}
	return fmt.Sprintf("%s %s for %s: %s", rule.ID, severity, target, reason)
}

func safeSummary(summary string) string {
	value := redaction.Redact(summary)
	if value == "" {
		return "rule evaluated"
	}
	if len(value) > 240 {
		return value[:240]
	}
	return value
}

func safeLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = redaction.Redact(value)
	}
	return out
}
