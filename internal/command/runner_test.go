package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	result, err := Run(context.Background(), helperSpec("success"))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Success() {
		t.Fatalf("Success() = false, result = %+v", result)
	}
	if result.Stdout != "hello\n" || result.Stderr != "note\n" {
		t.Fatalf("stdout/stderr = %q/%q", result.Stdout, result.Stderr)
	}
}

func TestRunNonZeroExit(t *testing.T) {
	result, err := Run(context.Background(), helperSpec("nonzero"))
	if err == nil {
		t.Fatal("Run() error = nil, want non-zero error")
	}
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("error type = %T, want *CommandError", err)
	}
	if result.ErrorClass != ErrorClassNonZeroExit || cmdErr.Class != ErrorClassNonZeroExit {
		t.Fatalf("class = %q/%q, want %q", result.ErrorClass, cmdErr.Class, ErrorClassNonZeroExit)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
}

func TestRunTimeout(t *testing.T) {
	spec := helperSpec("slow")
	spec.Timeout = 50 * time.Millisecond
	result, err := Run(context.Background(), spec)
	if err == nil {
		t.Fatal("Run() error = nil, want timeout")
	}
	if result.ErrorClass != ErrorClassTimeout {
		t.Fatalf("class = %q, want %q; error=%v", result.ErrorClass, ErrorClassTimeout, err)
	}
}

func TestRunOversizedOutput(t *testing.T) {
	spec := helperSpec("spamout")
	spec.MaxStdout = 64
	result, err := Run(context.Background(), spec)
	if err == nil {
		t.Fatal("Run() error = nil, want output limit")
	}
	if result.ErrorClass != ErrorClassOutputLimit {
		t.Fatalf("class = %q, want %q; error=%v", result.ErrorClass, ErrorClassOutputLimit, err)
	}
	if len(result.Stdout) > int(spec.MaxStdout) {
		t.Fatalf("stdout length = %d, limit = %d", len(result.Stdout), spec.MaxStdout)
	}
}

func TestRunMissingExecutable(t *testing.T) {
	spec := helperSpec("success")
	spec.Path = "/definitely/missing/pooly-agent-test-helper"
	result, err := Run(context.Background(), spec)
	if err == nil {
		t.Fatal("Run() error = nil, want missing executable")
	}
	if result.ErrorClass != ErrorClassMissingExecutable {
		t.Fatalf("class = %q, want %q; error=%v", result.ErrorClass, ErrorClassMissingExecutable, err)
	}
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	spec := helperSpec("slow")
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	result, err := Run(ctx, spec)
	if err == nil {
		t.Fatal("Run() error = nil, want cancellation")
	}
	if result.ErrorClass != ErrorClassCanceled {
		t.Fatalf("class = %q, want %q; error=%v", result.ErrorClass, ErrorClassCanceled, err)
	}
}

func TestRunRedactsCapturedOutputAndErrors(t *testing.T) {
	spec := helperSpec("secret")
	result, err := Run(context.Background(), spec)
	if err == nil {
		t.Fatal("Run() error = nil, want non-zero error")
	}
	combined := result.Stdout + result.Stderr + err.Error()
	for _, forbidden := range []string{webhookHostPath(), "redaction-test-token", "Bearer " + "abc123"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("secret %q leaked in %q", forbidden, combined)
		}
	}
	if !strings.Contains(combined, "[REDACTED]") {
		t.Fatalf("redacted output missing replacement marker: %q", combined)
	}
}

func TestValidateSpecRejectsShellStrings(t *testing.T) {
	spec := helperSpec("success")
	spec.Path = "sh"
	if err := ValidateSpec(spec); err == nil {
		t.Fatal("ValidateSpec() error = nil, want shell rejection")
	}

	spec = helperSpec("success")
	spec.Path = "/bin/echo hello"
	if err := ValidateSpec(spec); err == nil {
		t.Fatal("ValidateSpec() error = nil, want shell-string path rejection")
	}
}

func helperSpec(mode string) CommandSpec {
	return CommandSpec{
		Path:         os.Args[0],
		Args:         []string{"-test.run=TestCommandHelperProcess", "--", mode},
		Timeout:      2 * time.Second,
		MaxStdout:    4096,
		MaxStderr:    4096,
		RedactOutput: true,
	}
}

func TestCommandHelperProcess(t *testing.T) {
	if os.Getenv("POOLY_SENTINEL_COMMAND_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "success":
		fmt.Fprintln(os.Stdout, "hello")
		fmt.Fprintln(os.Stderr, "note")
	case "nonzero":
		fmt.Fprintln(os.Stderr, "bad exit")
		os.Exit(7)
	case "slow":
		time.Sleep(2 * time.Second)
	case "spamout":
		for i := 0; i < 1024; i++ {
			fmt.Fprint(os.Stdout, "0123456789")
		}
	case "secret":
		fmt.Fprintln(os.Stdout, fakeDiscordWebhook())
		fmt.Fprintln(os.Stderr, "Authorization: "+"Bearer "+"abc123")
		os.Exit(3)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q", mode)
		os.Exit(99)
	}
	os.Exit(0)
}

func fakeDiscordWebhook() string {
	return "https://" + webhookHostPath() + "/123/redaction-test-token"
}

func webhookHostPath() string {
	return "discord.com" + "/api/" + "webhooks"
}

func init() {
	if len(os.Args) > 0 {
		for _, arg := range os.Args {
			if strings.HasPrefix(arg, "-test.run=TestCommandHelperProcess") {
				_ = os.Setenv("POOLY_SENTINEL_COMMAND_HELPER", "1")
				return
			}
		}
	}
}
