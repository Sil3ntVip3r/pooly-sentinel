package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type ErrorClass string

const (
	ErrorClassNone              ErrorClass = ""
	ErrorClassValidation        ErrorClass = "validation"
	ErrorClassMissingExecutable ErrorClass = "missing_executable"
	ErrorClassStartFailed       ErrorClass = "start_failed"
	ErrorClassNonZeroExit       ErrorClass = "non_zero_exit"
	ErrorClassTimeout           ErrorClass = "timeout"
	ErrorClassCanceled          ErrorClass = "canceled"
	ErrorClassOutputLimit       ErrorClass = "output_limit"
)

type Result struct {
	Path       string
	Args       []string
	Stdout     string
	Stderr     string
	ExitCode   int
	Duration   time.Duration
	ErrorClass ErrorClass
}

func (r Result) Success() bool {
	return r.ErrorClass == ErrorClassNone && r.ExitCode == 0
}

type CommandError struct {
	Class    ErrorClass
	Path     string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{"command failed", "class=" + string(e.Class)}
	if e.Path != "" {
		parts = append(parts, "path="+e.Path)
	}
	if e.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit_code=%d", e.ExitCode))
	}
	if e.Err != nil {
		parts = append(parts, "error="+e.Err.Error())
	}
	if e.Stderr != "" {
		parts = append(parts, "stderr="+e.Stderr)
	}
	return redaction.Redact(strings.Join(parts, " "))
}
