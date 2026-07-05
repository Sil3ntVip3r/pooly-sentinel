package storage

import (
	"errors"
	"fmt"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type ErrorClass string

const (
	ErrorClassValidation ErrorClass = "validation"
	ErrorClassOpen       ErrorClass = "open"
	ErrorClassMigrate    ErrorClass = "migrate"
	ErrorClassQuery      ErrorClass = "query"
	ErrorClassWrite      ErrorClass = "write"
	ErrorClassNotFound   ErrorClass = "not_found"
	ErrorClassClosed     ErrorClass = "closed"
	ErrorClassFuture     ErrorClass = "future_schema"
)

var (
	ErrNotFound     = errors.New("storage record not found")
	ErrClosed       = errors.New("storage is closed")
	ErrFutureSchema = errors.New("database schema is newer than this binary supports")
)

type Error struct {
	Op    string
	Class ErrorClass
	Err   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return redaction.Redact(fmt.Sprintf("storage %s failed class=%s", e.Op, e.Class))
	}
	return redaction.Redact(fmt.Sprintf("storage %s failed class=%s: %v", e.Op, e.Class, e.Err))
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func wrapError(op string, class ErrorClass, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrClosed) || errors.Is(err, ErrFutureSchema) {
		return &Error{Op: op, Class: class, Err: err}
	}
	return &Error{Op: op, Class: class, Err: redaction.Error(err)}
}

func notFound(op string) error {
	return &Error{Op: op, Class: ErrorClassNotFound, Err: ErrNotFound}
}
