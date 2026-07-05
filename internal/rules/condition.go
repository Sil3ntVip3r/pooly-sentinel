package rules

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
)

type Operator string

const (
	OpGreaterThan        Operator = "greater_than"
	OpGreaterThanOrEqual Operator = "greater_than_or_equal"
	OpLessThan           Operator = "less_than"
	OpLessThanOrEqual    Operator = "less_than_or_equal"
	OpEqual              Operator = "equal"
	OpNotEqual           Operator = "not_equal"
	OpBooleanTrue        Operator = "boolean_true"
	OpBooleanFalse       Operator = "boolean_false"
	OpStateMatch         Operator = "state_match"
	OpEventCategoryMatch Operator = "event_category_match"
)

type Value struct {
	Number float64
	String string
	Bool   bool
	Kind   string
}

type Threshold struct {
	Operator Operator
	Value    Value
	For      time.Duration
}

type Rule struct {
	ID            string
	Enabled       bool
	Collector     string
	Metric        string
	Target        string
	EventCategory string
	Warn          *Threshold
	Fail          *Threshold
	Critical      *Threshold
	RecoverFor    time.Duration
	MissingData   Policy
	StaleData     Policy
	Summary       string
	Labels        map[string]string
}

type matchResult struct {
	Matched  bool
	Target   string
	Observed time.Time
	Metric   *resources.Metric
	Event    *resources.Event
}

func (r Rule) desiredSeverity(observations []resources.Observation) (Severity, string, time.Time, bool) {
	return r.desiredSeverityForTarget(observations, "")
}

func (r Rule) desiredSeverityForTarget(observations []resources.Observation, target string) (Severity, string, time.Time, bool) {
	matches := r.findMatches(observations)
	if target != "" {
		filtered := matches[:0]
		for _, match := range matches {
			if match.Target == target || (target == "system" && match.Target == "all") || (target == r.defaultTarget() && match.Target == "") {
				filtered = append(filtered, match)
			}
		}
		matches = filtered
	}
	if len(matches) == 0 {
		return SeverityNone, "metric or event was not observed", time.Time{}, false
	}
	var observed time.Time
	for _, match := range matches {
		if observed.IsZero() || match.Observed.After(observed) {
			observed = match.Observed
		}
		if r.Critical != nil && thresholdMatches(*r.Critical, match) {
			return SeverityCritical, "critical threshold matched", observed, true
		}
	}
	for _, match := range matches {
		if r.Fail != nil && thresholdMatches(*r.Fail, match) {
			return SeverityFailure, "failure threshold matched", observed, true
		}
	}
	for _, match := range matches {
		if r.Warn != nil && thresholdMatches(*r.Warn, match) {
			return SeverityWarning, "warning threshold matched", observed, true
		}
	}
	return SeverityNone, "condition is not true", observed, true
}

func (r Rule) matchingTargets(observations []resources.Observation) []string {
	if r.Target != "" && r.Target != "any" {
		return []string{r.Target}
	}
	seen := map[string]struct{}{}
	var targets []string
	for _, match := range r.findMatches(observations) {
		target := match.Target
		if target == "" {
			target = r.defaultTarget()
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		targets = append(targets, r.defaultTarget())
	}
	return targets
}

func (r Rule) findMatches(observations []resources.Observation) []matchResult {
	var matches []matchResult
	for _, observation := range observations {
		if !collectorMatches(r.Collector, observation.Collector) {
			continue
		}
		if r.Metric != "" {
			for i := range observation.Metrics {
				metric := &observation.Metrics[i]
				if metric.Name != r.Metric {
					continue
				}
				target := metricTarget(observation, *metric)
				if !targetMatches(r.Target, target) {
					continue
				}
				ts := metric.Timestamp
				if ts.IsZero() {
					ts = observation.Timestamp
				}
				matches = append(matches, matchResult{Matched: true, Target: target, Observed: ts.UTC(), Metric: metric})
			}
		}
		if r.EventCategory != "" {
			for i := range observation.Events {
				event := &observation.Events[i]
				if !targetMatches(r.Target, eventTarget(observation, *event)) {
					continue
				}
				if r.EventCategory != "" && event.Category != r.EventCategory {
					continue
				}
				ts := event.Timestamp
				if ts.IsZero() {
					ts = observation.Timestamp
				}
				matches = append(matches, matchResult{Matched: true, Target: eventTarget(observation, *event), Observed: ts.UTC(), Event: event})
			}
		}
	}
	return matches
}

func thresholdMatches(threshold Threshold, match matchResult) bool {
	switch threshold.Operator {
	case OpGreaterThan:
		return match.numeric() > threshold.Value.Number
	case OpGreaterThanOrEqual:
		return match.numeric() >= threshold.Value.Number
	case OpLessThan:
		return match.numeric() < threshold.Value.Number
	case OpLessThanOrEqual:
		return match.numeric() <= threshold.Value.Number
	case OpEqual:
		if threshold.Value.Kind == "string" {
			return strings.EqualFold(match.stringValue(), threshold.Value.String)
		}
		if threshold.Value.Kind == "bool" {
			return match.boolValue() == threshold.Value.Bool
		}
		return math.Abs(match.numeric()-threshold.Value.Number) < 1e-12
	case OpNotEqual:
		if threshold.Value.Kind == "string" {
			return !strings.EqualFold(match.stringValue(), threshold.Value.String)
		}
		if threshold.Value.Kind == "bool" {
			return match.boolValue() != threshold.Value.Bool
		}
		return math.Abs(match.numeric()-threshold.Value.Number) >= 1e-12
	case OpBooleanTrue:
		return match.boolValue()
	case OpBooleanFalse:
		return !match.boolValue()
	case OpStateMatch:
		return strings.EqualFold(match.stringValue(), threshold.Value.String)
	case OpEventCategoryMatch:
		return match.Event != nil && strings.EqualFold(match.Event.Category, threshold.Value.String)
	default:
		return false
	}
}

func (m matchResult) numeric() float64 {
	if m.Metric == nil {
		return 0
	}
	return m.Metric.Value
}

func (m matchResult) boolValue() bool {
	return m.numeric() != 0
}

func (m matchResult) stringValue() string {
	if m.Event != nil {
		return m.Event.Category
	}
	if m.Metric == nil {
		return ""
	}
	for _, key := range []string{"state", "status", "directive", "port", "unit", "interface", "mount", "device", "watch"} {
		if value := m.Metric.Labels[key]; value != "" {
			return value
		}
	}
	return fmt.Sprintf("%g", m.Metric.Value)
}

func metricTarget(observation resources.Observation, metric resources.Metric) string {
	for _, key := range []string{"mount", "interface", "device", "unit", "directive", "port", "watch", "collector"} {
		if value := metric.Labels[key]; value != "" {
			return value
		}
	}
	if observation.Target != "" {
		return observation.Target
	}
	return "system"
}

func eventTarget(observation resources.Observation, event resources.Event) string {
	for _, key := range []string{"unit", "stream", "event_category", "interface", "device", "watch"} {
		if value := event.Labels[key]; value != "" {
			return value
		}
	}
	if observation.Target != "" {
		return observation.Target
	}
	return "system"
}

func targetMatches(ruleTarget, observedTarget string) bool {
	if ruleTarget == "" || ruleTarget == "any" {
		return true
	}
	if ruleTarget == "system" && observedTarget == "all" {
		return true
	}
	return ruleTarget == observedTarget
}

func collectorMatches(ruleCollector, observationCollector string) bool {
	if ruleCollector == observationCollector {
		return true
	}
	if ruleCollector == "resources" {
		switch observationCollector {
		case "cpu", "load", "memory", "pressure", "filesystem", "diskio", "network", "uptime":
			return true
		}
	}
	return false
}

func (r Rule) defaultTarget() string {
	if r.Target != "" && r.Target != "any" {
		return r.Target
	}
	return "system"
}
