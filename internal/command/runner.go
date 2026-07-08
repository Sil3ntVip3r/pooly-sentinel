package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type CommandSpec struct {
	Path         string
	Args         []string
	Timeout      time.Duration
	MaxStdout    int64
	MaxStderr    int64
	RedactOutput bool
}

var errOutputLimit = errors.New("command output limit exceeded")

func Run(ctx context.Context, spec CommandSpec) (Result, error) {
	started := time.Now()
	result := Result{
		Path:     spec.Path,
		Args:     append([]string(nil), spec.Args...),
		ExitCode: -1,
	}

	if ctx == nil {
		result.ErrorClass = ErrorClassValidation
		return result, commandError(result, ErrorClassValidation, 0, "", fmt.Errorf("context is nil"))
	}
	if err := ValidateSpec(spec); err != nil {
		result.ErrorClass = ErrorClassValidation
		return result, commandError(result, ErrorClassValidation, 0, "", err)
	}
	if err := ctx.Err(); err != nil {
		result.ErrorClass = classifyContextError(err)
		return result, commandError(result, result.ErrorClass, 0, "", err)
	}

	resolvedPath, err := resolveExecutable(spec.Path)
	if err != nil {
		class := ErrorClassMissingExecutable
		if !errors.Is(err, os.ErrNotExist) {
			class = ErrorClassStartFailed
		}
		result.ErrorClass = class
		return result, commandError(result, class, 0, "", err)
	}

	timeoutCtx := ctx
	cancelTimeout := func() {}
	if spec.Timeout > 0 {
		timeoutCtx, cancelTimeout = context.WithTimeout(ctx, spec.Timeout)
	}
	defer cancelTimeout()

	runCtx, cancelRun := context.WithCancel(timeoutCtx)
	defer cancelRun()

	cmd := exec.CommandContext(runCtx, resolvedPath, spec.Args...)
	configureCommandProcess(cmd)
	cmd.Cancel = func() error {
		if err := terminateCommandProcessGroup(cmd); err == nil || errors.Is(err, os.ErrProcessDone) {
			return err
		}
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = commandWaitDelay(spec.Timeout)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		result.ErrorClass = ErrorClassStartFailed
		return result, commandError(result, ErrorClassStartFailed, 0, "", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		result.ErrorClass = ErrorClassStartFailed
		return result, commandError(result, ErrorClassStartFailed, 0, "", err)
	}

	stdout := newLimitedBuffer(spec.MaxStdout)
	stderr := newLimitedBuffer(spec.MaxStderr)
	if err := cmd.Start(); err != nil {
		class := ErrorClassStartFailed
		if errors.Is(err, os.ErrNotExist) {
			class = ErrorClassMissingExecutable
		}
		result.ErrorClass = class
		return result, commandError(result, class, 0, "", err)
	}

	copyDone := make(chan streamCopyResult, 2)
	go copyStream(copyDone, "stdout", stdout, stdoutPipe, cancelRun)
	go copyStream(copyDone, "stderr", stderr, stderrPipe, cancelRun)

	waitErr := cmd.Wait()
	stdoutCopy := <-copyDone
	stderrCopy := <-copyDone

	result.Duration = time.Since(started)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if spec.RedactOutput {
		result.Stdout = redaction.Redact(result.Stdout)
		result.Stderr = redaction.Redact(result.Stderr)
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if stdoutCopy.exceeded || stderrCopy.exceeded {
		result.ErrorClass = ErrorClassOutputLimit
		return result, commandError(result, ErrorClassOutputLimit, result.ExitCode, result.Stderr, errOutputLimit)
	}
	if copyErr := firstCopyError(stdoutCopy.err, stderrCopy.err); copyErr != nil {
		result.ErrorClass = ErrorClassStartFailed
		return result, commandError(result, ErrorClassStartFailed, result.ExitCode, result.Stderr, copyErr)
	}
	if waitErr != nil {
		class := classifyWaitError(ctx, timeoutCtx, waitErr)
		result.ErrorClass = class
		return result, commandError(result, class, result.ExitCode, result.Stderr, waitErr)
	}
	if err := timeoutCtx.Err(); err != nil {
		class := classifyContextError(err)
		result.ErrorClass = class
		return result, commandError(result, class, result.ExitCode, result.Stderr, err)
	}

	result.ErrorClass = ErrorClassNone
	return result, nil
}

func commandWaitDelay(timeout time.Duration) time.Duration {
	defaultDelay := 2 * time.Second
	if timeout > 0 && timeout < defaultDelay {
		return timeout
	}
	return defaultDelay
}

func commandError(result Result, class ErrorClass, exitCode int, stderr string, err error) error {
	return &CommandError{
		Class:    class,
		Path:     result.Path,
		ExitCode: exitCode,
		Stderr:   stderr,
		Err:      redaction.Error(err),
	}
}

func classifyWaitError(parent context.Context, timeoutCtx context.Context, err error) ErrorClass {
	if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) || errors.Is(parent.Err(), context.DeadlineExceeded) {
		return ErrorClassTimeout
	}
	if errors.Is(parent.Err(), context.Canceled) || errors.Is(timeoutCtx.Err(), context.Canceled) {
		return ErrorClassCanceled
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return ErrorClassNonZeroExit
	}
	return ErrorClassStartFailed
}

func classifyContextError(err error) ErrorClass {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorClassTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrorClassCanceled
	}
	return ErrorClassStartFailed
}

type streamCopyResult struct {
	name     string
	err      error
	exceeded bool
}

func copyStream(done chan<- streamCopyResult, name string, dst *limitedBuffer, src io.Reader, cancel context.CancelFunc) {
	_, err := io.Copy(dst, src)
	exceeded := errors.Is(err, errOutputLimit) || dst.Exceeded()
	if exceeded {
		cancel()
		err = nil
	}
	done <- streamCopyResult{name: name, err: err, exceeded: exceeded}
}

func firstCopyError(errs ...error) error {
	for _, err := range errs {
		if err != nil && !errors.Is(err, os.ErrClosed) {
			return err
		}
	}
	return nil
}

type limitedBuffer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	limit    int64
	exceeded bool
}

func newLimitedBuffer(limit int64) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	remaining := b.limit - int64(b.buf.Len())
	if remaining <= 0 {
		b.exceeded = true
		return 0, errOutputLimit
	}
	if int64(len(p)) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.exceeded = true
		return int(remaining), errOutputLimit
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *limitedBuffer) Exceeded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exceeded
}
