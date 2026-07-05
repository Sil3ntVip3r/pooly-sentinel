package storage

import (
	"encoding/json"
	"fmt"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

func sanitizedJSONBytes(value any, indent bool) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	sanitized := sanitizeJSONValue(decoded, "")
	if indent {
		return json.MarshalIndent(sanitized, "", "  ")
	}
	return json.Marshal(sanitized)
}

func sanitizeJSONValue(value any, key string) any {
	if redaction.IsSensitiveKey(key) {
		return redaction.Replacement
	}
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for childKey, childValue := range v {
			out[childKey] = sanitizeJSONValue(childValue, childKey)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = sanitizeJSONValue(child, key)
		}
		return out
	case string:
		return redaction.Redact(v)
	default:
		return v
	}
}

func mapForJSON(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("mapForJSON requires an even number of key/value arguments")
	}
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok || key == "" {
			return nil, fmt.Errorf("mapForJSON key must be a non-empty string")
		}
		out[key] = pairs[i+1]
	}
	return out, nil
}
