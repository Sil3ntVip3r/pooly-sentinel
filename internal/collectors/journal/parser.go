package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type Record struct {
	Timestamp time.Time
	Cursor    string
	Priority  string
	Transport string
	Unit      string
	Command   string
	Category  string
	Summary   string
}

type ParseOptions struct {
	Stream        string
	MaxRecords    int
	MaxFieldBytes int
}

func ParseJSONLines(data []byte, opts ParseOptions) ([]Record, string, bool, error) {
	if opts.MaxRecords <= 0 {
		opts.MaxRecords = 100
	}
	if opts.MaxFieldBytes <= 0 {
		opts.MaxFieldBytes = 512
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), max(64*1024, opts.MaxFieldBytes*16))
	var records []Record
	var lastCursor string
	truncated := false
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if len(records) >= opts.MaxRecords {
			truncated = true
			break
		}
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			return nil, lastCursor, truncated, fmt.Errorf("journal record is malformed JSON")
		}
		record := normalizeRecord(raw, opts)
		if record.Cursor != "" {
			lastCursor = record.Cursor
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, lastCursor, truncated, fmt.Errorf("journal output scan failed")
	}
	return records, lastCursor, truncated, nil
}

func normalizeRecord(raw map[string]any, opts ParseOptions) Record {
	field := func(name string) string {
		return safeField(rawString(raw[name]), opts.MaxFieldBytes)
	}
	message := safeField(rawString(raw["MESSAGE"]), opts.MaxFieldBytes)
	unit := firstNonEmpty(field("_SYSTEMD_UNIT"), field("UNIT"))
	command := firstNonEmpty(field("_COMM"), field("SYSLOG_IDENTIFIER"), field("_EXE"))
	category := categorize(opts.Stream, message, unit, command)
	ts := parseRealtime(field("__REALTIME_TIMESTAMP"))
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	return Record{
		Timestamp: ts,
		Cursor:    field("__CURSOR"),
		Priority:  field("PRIORITY"),
		Transport: field("_TRANSPORT"),
		Unit:      safeIdentifier(unit),
		Command:   safeIdentifier(command),
		Category:  category,
		Summary:   safeSummary(category, unit, command),
	}
}

func rawString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

func safeField(value string, maxBytes int) string {
	value = redaction.Redact(strings.TrimSpace(value))
	if maxBytes <= 0 {
		maxBytes = 512
	}
	if len(value) > maxBytes {
		return value[:maxBytes]
	}
	return value
}

func safeIdentifier(value string) string {
	value = strings.TrimSpace(redaction.Redact(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == '@' {
			b.WriteRune(r)
		}
		if b.Len() >= 96 {
			break
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func parseRealtime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	micros, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(0, micros*1000).UTC()
}

func categorize(stream string, message string, unit string, command string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "failed password") || strings.Contains(lower, "authentication failure"):
		return "authentication_failure"
	case strings.Contains(lower, "accepted password") || strings.Contains(lower, "accepted publickey") || strings.Contains(lower, "session opened"):
		return "authentication_success"
	case strings.Contains(lower, "invalid user"):
		return "invalid_user"
	case strings.Contains(lower, "sudo") && (strings.Contains(lower, "authentication failure") || strings.Contains(lower, "incorrect password")):
		return "sudo_failure"
	case strings.Contains(lower, "out of memory") || strings.Contains(lower, "oom-killer") || strings.Contains(lower, "killed process"):
		return "kernel_oom"
	case strings.Contains(lower, "ext4") || strings.Contains(lower, "xfs") || strings.Contains(lower, "i/o error") || strings.Contains(lower, "blk_update_request"):
		return "kernel_storage"
	case strings.Contains(lower, "link is down") || strings.Contains(lower, "link is up") || strings.Contains(lower, "renamed from"):
		return "kernel_network"
	case strings.Contains(lower, "failed") && unit != "":
		return "service_failure"
	case strings.Contains(lower, "started") && unit != "":
		return "service_start"
	case strings.Contains(lower, "stopped") && unit != "":
		return "service_stop"
	case strings.Contains(lower, "restart") && unit != "":
		return "service_restart"
	case stream == "kernel":
		return "kernel_event"
	case stream == "auth":
		return "auth_event"
	case stream == "services":
		return "service_event"
	default:
		_ = command
		return "journal_event"
	}
}

func safeSummary(category string, unit string, command string) string {
	target := firstNonEmpty(unit, command)
	if target == "" {
		return category
	}
	return category + " from " + safeIdentifier(target)
}

func eventFromRecord(stream string, record Record) resources.Event {
	fields := map[string]string{
		"priority":  record.Priority,
		"transport": record.Transport,
	}
	if record.Unit != "" {
		fields["unit"] = record.Unit
	}
	if record.Command != "" {
		fields["command"] = record.Command
	}
	return resources.Event{
		Category:  record.Category,
		Timestamp: record.Timestamp.UTC(),
		Summary:   record.Summary,
		Labels: map[string]string{
			"stream":         stream,
			"event_category": record.Category,
		},
		Fields: fields,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
