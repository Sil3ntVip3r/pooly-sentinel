package logging

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type RedactingHandler struct {
	next slog.Handler
}

func NewRedactingHandler(next slog.Handler) *RedactingHandler {
	return &RedactingHandler{next: next}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, redaction.Redact(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, redacted)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, redactAttr(attr))
	}
	return &RedactingHandler{next: h.next.WithAttrs(redacted)}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(redaction.Redact(name))}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if attr.Key != "" && redaction.IsSensitiveKey(attr.Key) {
		return slog.String(attr.Key, redaction.Replacement)
	}

	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(redaction.Redact(value.String()))
	case slog.KindGroup:
		children := value.Group()
		redactedChildren := make([]slog.Attr, 0, len(children))
		for _, child := range children {
			redactedChildren = append(redactedChildren, redactAttr(child))
		}
		attr.Value = slog.GroupValue(redactedChildren...)
	case slog.KindAny:
		switch v := value.Any().(type) {
		case error:
			attr.Value = slog.StringValue(redaction.Redact(v.Error()))
		case fmt.Stringer:
			attr.Value = slog.StringValue(redaction.Redact(v.String()))
		case []byte:
			attr.Value = slog.StringValue(redaction.Redact(string(v)))
		default:
			attr.Value = value
		}
	default:
		attr.Value = value
	}
	return attr
}
