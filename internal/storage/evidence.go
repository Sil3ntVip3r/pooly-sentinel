package storage

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type EvidenceWriter struct {
	LogDir string
}

func NewEvidenceWriter(logDir string) EvidenceWriter {
	return EvidenceWriter{LogDir: logDir}
}

func (w EvidenceWriter) WriteText(ctx context.Context, incidentID string, filename string, content string) (string, error) {
	path, err := w.evidencePath(incidentID, filename)
	if err != nil {
		return "", wrapError("evidence path", ErrorClassValidation, err)
	}
	if err := ensureDirNoSymlinkUnder(filepath.Clean(w.LogDir), filepath.Dir(path)); err != nil {
		return "", wrapError("evidence mkdir", ErrorClassWrite, err)
	}
	data := []byte(redaction.Redact(content))
	if err := atomicWriteFile(ctx, path, data); err != nil {
		return "", wrapError("evidence write", ErrorClassWrite, err)
	}
	return path, nil
}

func (w EvidenceWriter) WriteJSON(ctx context.Context, incidentID string, filename string, value any) (string, error) {
	path, err := w.evidencePath(incidentID, filename)
	if err != nil {
		return "", wrapError("evidence path", ErrorClassValidation, err)
	}
	if err := ensureDirNoSymlinkUnder(filepath.Clean(w.LogDir), filepath.Dir(path)); err != nil {
		return "", wrapError("evidence mkdir", ErrorClassWrite, err)
	}
	data, err := sanitizedJSONBytes(value, true)
	if err != nil {
		return "", wrapError("evidence encode", ErrorClassWrite, err)
	}
	data = append(data, '\n')
	if err := atomicWriteFile(ctx, path, data); err != nil {
		return "", wrapError("evidence write", ErrorClassWrite, err)
	}
	return path, nil
}

func (w EvidenceWriter) evidencePath(incidentID string, filename string) (string, error) {
	if w.LogDir == "" {
		return "", fmt.Errorf("log directory is required")
	}
	safeIncidentID, err := sanitizeSegment(incidentID)
	if err != nil {
		return "", err
	}
	if err := validatePlainFilename(filename); err != nil {
		return "", err
	}
	safeFilename, err := sanitizeSegment(filename)
	if err != nil {
		return "", err
	}
	root, err := secureJoin(filepath.Clean(w.LogDir), "incidents", "open", safeIncidentID)
	if err != nil {
		return "", err
	}
	return secureJoin(root, safeFilename)
}
