package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type EventWriterOptions struct {
	Path          string
	MaxEventBytes int
	SyncOnWrite   bool
}

type EventWriter struct {
	mu            sync.Mutex
	file          *os.File
	path          string
	maxEventBytes int
	syncOnWrite   bool
	closed        bool
}

func OpenEventWriter(ctx context.Context, opts EventWriterOptions) (*EventWriter, error) {
	if ctx == nil {
		return nil, wrapError("open event writer", ErrorClassValidation, fmt.Errorf("context is nil"))
	}
	if opts.Path == "" {
		return nil, wrapError("open event writer", ErrorClassValidation, fmt.Errorf("path is required"))
	}
	if opts.MaxEventBytes <= 0 {
		opts.MaxEventBytes = DefaultMaxEventBytes
	}
	if err := ctx.Err(); err != nil {
		return nil, wrapError("open event writer", ErrorClassWrite, err)
	}
	if err := ensureDirNoSymlink(filepath.Dir(opts.Path)); err != nil {
		return nil, wrapError("open event writer mkdir", ErrorClassWrite, err)
	}
	file, err := openRegularNoFollow(opts.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, FileMode)
	if err != nil {
		return nil, wrapError("open event writer", ErrorClassWrite, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, wrapError("stat event writer", ErrorClassWrite, err)
	}
	if !fileModeIsRestrictive(info.Mode()) {
		_ = file.Close()
		return nil, wrapError("open event writer permissions", ErrorClassValidation, fmt.Errorf("event file permissions are too permissive"))
	}
	return &EventWriter{
		file:          file,
		path:          opts.Path,
		maxEventBytes: opts.MaxEventBytes,
		syncOnWrite:   opts.SyncOnWrite,
	}, nil
}

func (w *EventWriter) Write(ctx context.Context, event any) error {
	if ctx == nil {
		return wrapError("write event", ErrorClassValidation, fmt.Errorf("context is nil"))
	}
	data, err := sanitizedJSONBytes(event, false)
	if err != nil {
		return wrapError("write event encode", ErrorClassWrite, err)
	}
	if len(data)+1 > w.maxEventBytes {
		return wrapError("write event oversized", ErrorClassValidation, fmt.Errorf("event size %d exceeds maximum %d bytes", len(data)+1, w.maxEventBytes))
	}
	data = append(data, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.file == nil {
		return wrapError("write event", ErrorClassClosed, ErrClosed)
	}
	if err := ctx.Err(); err != nil {
		return wrapError("write event", ErrorClassWrite, err)
	}
	if _, err := w.file.Write(data); err != nil {
		return wrapError("write event", ErrorClassWrite, err)
	}
	if w.syncOnWrite {
		if err := w.file.Sync(); err != nil {
			return wrapError("sync event", ErrorClassWrite, err)
		}
	}
	return nil
}

func (w *EventWriter) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return wrapError("close event writer", ErrorClassClosed, err)
	}
	return nil
}
