package notify

import (
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

func RenderPayload(incident storage.IncidentRecord, event Event) Payload {
	return Payload{
		Event:           event,
		IncidentID:      redaction.Redact(incident.ID),
		Fingerprint:     redaction.Redact(incident.Fingerprint),
		NodeID:          redaction.Redact(incident.NodeID),
		Type:            redaction.Redact(incident.Type),
		Target:          redaction.Redact(incident.Target),
		Condition:       redaction.Redact(incident.Condition),
		Severity:        redaction.Redact(incident.Severity),
		Status:          redaction.Redact(incident.Status),
		Summary:         safeSummary(incident.Summary),
		FirstSeen:       incident.FirstSeen.UTC(),
		LastSeen:        incident.LastSeen.UTC(),
		OccurrenceCount: incident.OccurrenceCount,
		LastTransition:  utcPtr(incident.LastTransition),
		ResolvedAt:      utcPtr(incident.ResolvedAt),
		EvidencePath:    safeEvidencePath(incident.EvidencePath),
	}
}

func safeSummary(summary string) string {
	value := redaction.Redact(summary)
	if value == "" {
		return "incident lifecycle event"
	}
	if len(value) > 500 {
		return value[:500]
	}
	return value
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func safeEvidencePath(path string) string {
	if path == "" {
		return ""
	}
	if redaction.Redact(path) != path {
		return ""
	}
	if strings.ContainsAny(path, "\x00\r\n\t") {
		return ""
	}
	if parsed, err := url.Parse(path); err == nil && parsed.Scheme != "" {
		return ""
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return ""
		}
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return ""
	}
	return clean
}
