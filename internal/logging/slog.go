package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

const (
	FieldComponent  = "component"
	FieldCollector  = "collector"
	FieldRuleID     = "rule_id"
	FieldIncident   = "incident_id"
	FieldSeverity   = "severity"
	FieldDuration   = "duration"
	FieldErrorClass = "error_class"
)

type Options struct {
	Level     string
	Format    string
	AddSource bool
}

func New(out io.Writer, opts Options) (*slog.Logger, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	handlerOptions := &slog.HandlerOptions{
		AddSource: opts.AddSource,
		Level:     level,
	}

	var handler slog.Handler
	switch strings.ToLower(opts.Format) {
	case "", "text":
		handler = slog.NewTextHandler(out, handlerOptions)
	case "json":
		handler = slog.NewJSONHandler(out, handlerOptions)
	default:
		return nil, fmt.Errorf("unsupported log format %q", opts.Format)
	}

	return slog.New(NewRedactingHandler(handler)), nil
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level %q", level)
	}
}

func Component(value string) slog.Attr {
	return slog.String(FieldComponent, value)
}

func Collector(value string) slog.Attr {
	return slog.String(FieldCollector, value)
}

func RuleID(value string) slog.Attr {
	return slog.String(FieldRuleID, value)
}

func IncidentID(value string) slog.Attr {
	return slog.String(FieldIncident, value)
}

func Severity(value string) slog.Attr {
	return slog.String(FieldSeverity, value)
}

func ErrorClass(value string) slog.Attr {
	return slog.String(FieldErrorClass, value)
}
