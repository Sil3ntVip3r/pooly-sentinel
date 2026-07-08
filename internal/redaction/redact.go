package redaction

import (
	"errors"
	"fmt"
	"strings"
)

// Redact removes sensitive values from text that may be logged, printed, or
// returned as an error. It is intentionally conservative.
func Redact(s string) string {
	if s == "" {
		return s
	}

	out := privateKeyPattern.ReplaceAllString(s, Replacement)
	out = discordWebhookPattern.ReplaceAllString(out, Replacement)
	out = genericWebhookTokenURLPattern.ReplaceAllString(out, Replacement)
	out = authorizedKeyPattern.ReplaceAllString(out, Replacement)
	out = authorizationPattern.ReplaceAllString(out, "${1}"+Replacement)
	out = querySecretPattern.ReplaceAllString(out, "${1}"+Replacement)
	out = keyValuePattern.ReplaceAllString(out, "${1}${2}"+Replacement)
	return out
}

// Value formats and redacts a value. It is useful at package boundaries where
// an unknown value might otherwise flow into logs.
func Value(v any) string {
	return Redact(fmt.Sprint(v))
}

// Error returns an error whose formatted text has been redacted.
func Error(err error) error {
	if err == nil {
		return nil
	}
	return redactedError{msg: Redact(err.Error())}
}

// NewError creates a redacted error from a message.
func NewError(msg string) error {
	if msg == "" {
		return errors.New("")
	}
	return redactedError{msg: Redact(msg)}
}

// IsSensitiveKey reports whether a structured log/config key should have its
// value redacted even if the value itself does not match a known token shape.
func IsSensitiveKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
	for _, needle := range []string{
		"token",
		"secret",
		"password",
		"passwd",
		"pwd",
		"apikey",
		"authorization",
		"webhook",
		"privatekey",
		"authorizedkeys",
	} {
		if strings.Contains(normalized, needle) {
			return true
		}
	}
	return false
}

type redactedError struct {
	msg string
}

func (e redactedError) Error() string {
	return e.msg
}
