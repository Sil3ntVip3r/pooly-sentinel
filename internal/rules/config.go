package rules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/config"
)

func FromConfig(cfg config.Config) ([]Rule, error) {
	rules := make([]Rule, 0, len(cfg.Rules))
	for _, item := range cfg.Rules {
		rule := Rule{
			ID:            item.ID,
			Enabled:       item.Enabled,
			Collector:     item.Collector,
			Metric:        item.Metric,
			Target:        item.Target,
			EventCategory: item.EventCategory,
			RecoverFor:    item.RecoverFor.Duration,
			MissingData:   policyOrDefault(item.MissingData, PolicyStale),
			StaleData:     policyOrDefault(item.StaleData, PolicyStale),
			Summary:       item.Summary,
			Labels:        item.Labels,
		}
		var err error
		rule.Warn, err = thresholdFromConfig(item.Warn)
		if err != nil {
			return nil, fmt.Errorf("rule %s warn: %w", item.ID, err)
		}
		rule.Fail, err = thresholdFromConfig(item.Fail)
		if err != nil {
			return nil, fmt.Errorf("rule %s fail: %w", item.ID, err)
		}
		rule.Critical, err = thresholdFromConfig(item.Critical)
		if err != nil {
			return nil, fmt.Errorf("rule %s critical: %w", item.ID, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func thresholdFromConfig(cfg *config.RuleThresholdConfig) (*Threshold, error) {
	if cfg == nil {
		return nil, nil
	}
	value, err := valueFromConfig(cfg.Value)
	if err != nil {
		return nil, err
	}
	return &Threshold{
		Operator: Operator(cfg.Operator),
		Value:    value,
		For:      cfg.For.Duration,
	}, nil
}

func valueFromConfig(value any) (Value, error) {
	switch v := value.(type) {
	case nil:
		return Value{}, nil
	case int:
		return Value{Number: float64(v), Kind: "number"}, nil
	case int64:
		return Value{Number: float64(v), Kind: "number"}, nil
	case float64:
		return Value{Number: v, Kind: "number"}, nil
	case float32:
		return Value{Number: float64(v), Kind: "number"}, nil
	case bool:
		return Value{Bool: v, Kind: "bool"}, nil
	case string:
		text := strings.TrimSpace(v)
		if number, err := strconv.ParseFloat(text, 64); err == nil {
			return Value{Number: number, String: text, Kind: "number"}, nil
		}
		return Value{String: text, Kind: "string"}, nil
	default:
		return Value{}, fmt.Errorf("unsupported threshold value type %T", value)
	}
}

func policyOrDefault(value string, fallback Policy) Policy {
	if value == "" {
		return fallback
	}
	return Policy(value)
}
