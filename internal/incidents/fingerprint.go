package incidents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

const (
	MaxFingerprintComponentLength = 128
	MaxFingerprintLength          = 512
)

var fingerprintComponentPattern = regexp.MustCompile(`^[a-z0-9._:@/-]+$`)

func Fingerprint(nodeID string, incidentType string, target string, condition string) (string, error) {
	parts := []string{nodeID, incidentType, target, condition}
	normalized := make([]string, 0, len(parts))
	for i, part := range parts {
		value, err := normalizeFingerprintComponent(part)
		if err != nil {
			names := []string{"node_id", "type", "target", "condition"}
			return "", fmt.Errorf("%s: %w", names[i], err)
		}
		normalized = append(normalized, value)
	}
	fingerprint := strings.Join(normalized, ":")
	if len(fingerprint) > MaxFingerprintLength {
		return "", fmt.Errorf("fingerprint exceeds %d bytes", MaxFingerprintLength)
	}
	if redaction.Redact(fingerprint) != fingerprint {
		return "", fmt.Errorf("fingerprint contains sensitive material")
	}
	return fingerprint, nil
}

func IncidentIDForFingerprint(fingerprint string) (string, error) {
	if strings.TrimSpace(fingerprint) == "" {
		return "", fmt.Errorf("fingerprint is required")
	}
	if len(fingerprint) > MaxFingerprintLength {
		return "", fmt.Errorf("fingerprint exceeds %d bytes", MaxFingerprintLength)
	}
	sum := sha256.Sum256([]byte(fingerprint))
	return "inc_" + hex.EncodeToString(sum[:16]), nil
}

func normalizeFingerprintComponent(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", fmt.Errorf("is required")
	}
	if len(normalized) > MaxFingerprintComponentLength {
		return "", fmt.Errorf("exceeds %d bytes", MaxFingerprintComponentLength)
	}
	if strings.ContainsAny(normalized, "\x00\r\n\t ") {
		return "", fmt.Errorf("contains unsafe whitespace or control characters")
	}
	if strings.Contains(normalized, "..") {
		return "", fmt.Errorf("contains unsafe traversal marker")
	}
	if !fingerprintComponentPattern.MatchString(normalized) {
		return "", fmt.Errorf("contains unsupported characters")
	}
	if redaction.Redact(normalized) != normalized {
		return "", fmt.Errorf("contains sensitive material")
	}
	return normalized, nil
}
