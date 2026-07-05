package systemd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/platform"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/command"
)

type fakeRunner struct {
	result command.Result
	err    error
	spec   command.CommandSpec
}

func (f *fakeRunner) Run(ctx context.Context, spec command.CommandSpec) (command.Result, error) {
	f.spec = spec
	return f.result, f.err
}

func TestParseShowOutputStates(t *testing.T) {
	cases := []struct {
		name        string
		activeState string
		loadState   string
		summary     string
	}{
		{name: "active", activeState: "active", loadState: "loaded", summary: "unit active"},
		{name: "inactive", activeState: "inactive", loadState: "loaded", summary: "unit inactive"},
		{name: "failed", activeState: "failed", loadState: "loaded", summary: "unit failed state observed"},
		{name: "activating", activeState: "activating", loadState: "loaded", summary: "unit activating"},
		{name: "missing", activeState: "inactive", loadState: "not-found", summary: "unit missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := strings.Join([]string{
				"Id=ssh.service",
				"LoadState=" + tc.loadState,
				"ActiveState=" + tc.activeState,
				"SubState=running",
				"UnitFileState=enabled",
				"Result=success",
				"MainPID=123",
				"ExecMainCode=0",
				"ExecMainStatus=0",
				"NRestarts=2",
				"ActiveEnterTimestampMonotonic=4000000",
			}, "\n")
			runner := &fakeRunner{result: command.Result{Stdout: output, ExitCode: 0}}
			obs := Collect(context.Background(), Options{
				PlatformSupported: platform.Bool(true),
				Services:          []string{"ssh.service"},
				Runner:            runner,
			})
			if len(obs) != 1 || !obs[0].Success {
				t.Fatalf("Collect() = %+v", obs)
			}
			if obs[0].Summary != tc.summary {
				t.Fatalf("summary = %q, want %q", obs[0].Summary, tc.summary)
			}
			if runner.spec.Args[0] != "show" || strings.Contains(strings.Join(runner.spec.Args, " "), "status") {
				t.Fatalf("unsafe systemctl args: %#v", runner.spec.Args)
			}
		})
	}
}

func TestCollectUnitMalformedAndTimeout(t *testing.T) {
	malformed := &fakeRunner{result: command.Result{Stdout: "LoadState loaded\n"}}
	obs := Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Services: []string{"bad.service"}, Runner: malformed})
	if len(obs) != 1 || obs[0].ErrorClass != resources.ErrorParse {
		t.Fatalf("malformed obs = %+v", obs)
	}

	timeout := &fakeRunner{err: &command.CommandError{Class: command.ErrorClassTimeout, Err: errors.New("deadline")}}
	obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Services: []string{"slow.service"}, Runner: timeout})
	if len(obs) != 1 || obs[0].ErrorClass != resources.ErrorTimeout {
		t.Fatalf("timeout obs = %+v", obs)
	}
}

func TestCollectUnitCommandErrorsWithParseableStdout(t *testing.T) {
	activeOutput := strings.Join([]string{
		"Id=ssh.service",
		"LoadState=loaded",
		"ActiveState=active",
		"SubState=running",
		"MainPID=123",
		"ExecMainCode=1",
		"ExecMainStatus=2",
		"NRestarts=3",
		"ActiveEnterTimestampMonotonic=4000000",
	}, "\n")
	missingOutput := strings.Join([]string{
		"Id=missing.service",
		"LoadState=not-found",
		"ActiveState=inactive",
		"SubState=dead",
		"MainPID=0",
		"NRestarts=0",
		"ActiveEnterTimestampMonotonic=0",
	}, "\n")
	cases := []struct {
		name      string
		output    string
		err       error
		wantClass resources.ErrorClass
		wantOK    bool
	}{
		{name: "timeout", output: activeOutput, err: &command.CommandError{Class: command.ErrorClassTimeout, Err: errors.New("deadline")}, wantClass: resources.ErrorTimeout},
		{name: "canceled", output: activeOutput, err: &command.CommandError{Class: command.ErrorClassCanceled, Err: errors.New("canceled")}, wantClass: resources.ErrorTimeout},
		{name: "output limit", output: activeOutput, err: &command.CommandError{Class: command.ErrorClassOutputLimit, Err: errors.New("limit")}, wantClass: resources.ErrorParse},
		{name: "missing executable", output: activeOutput, err: &command.CommandError{Class: command.ErrorClassMissingExecutable, Err: errors.New("missing")}, wantClass: resources.ErrorSourceMissing},
		{name: "permission", output: activeOutput, err: &command.CommandError{Class: command.ErrorClassNonZeroExit, Stderr: "Permission denied", Err: errors.New("permission")}, wantClass: resources.ErrorPermissionDenied},
		{name: "ordinary nonzero", output: activeOutput, err: &command.CommandError{Class: command.ErrorClassNonZeroExit, Err: errors.New("exit")}, wantClass: resources.ErrorCommand},
		{name: "ordinary nonzero malformed", output: "LoadState loaded\n", err: &command.CommandError{Class: command.ErrorClassNonZeroExit, Err: errors.New("exit")}, wantClass: resources.ErrorParse},
		{name: "missing unit nonzero", output: missingOutput, err: &command.CommandError{Class: command.ErrorClassNonZeroExit, Err: errors.New("exit")}, wantOK: true},
		{name: "missing unit timeout remains failure", output: missingOutput, err: &command.CommandError{Class: command.ErrorClassTimeout, Err: errors.New("deadline")}, wantClass: resources.ErrorTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{result: command.Result{Stdout: tc.output, ExitCode: 1}, err: tc.err}
			obs := Collect(context.Background(), Options{
				PlatformSupported: platform.Bool(true),
				Services:          []string{"ssh.service"},
				Runner:            runner,
			})
			if len(obs) != 1 {
				t.Fatalf("obs = %+v", obs)
			}
			if tc.wantOK {
				if !obs[0].Success || obs[0].Summary != "unit missing" {
					t.Fatalf("obs = %+v", obs)
				}
				return
			}
			if obs[0].Success || obs[0].ErrorClass != tc.wantClass {
				t.Fatalf("obs = %+v, want class %s", obs, tc.wantClass)
			}
		})
	}
}

func TestParseShowOutputRejectsNegativePropertiesAndExposesExecFields(t *testing.T) {
	for _, line := range []string{"MainPID=-1", "NRestarts=-1", "ActiveEnterTimestampMonotonic=-1"} {
		output := strings.Join([]string{
			"Id=ssh.service",
			"LoadState=loaded",
			"ActiveState=active",
			"SubState=running",
			line,
		}, "\n")
		if _, err := ParseShowOutput("ssh.service", output); err == nil {
			t.Fatalf("ParseShowOutput(%q) error = nil", line)
		}
	}
	output := strings.Join([]string{
		"Id=ssh.service",
		"LoadState=loaded",
		"ActiveState=active",
		"SubState=running",
		"ExecMainCode=7",
		"ExecMainStatus=9",
		"NRestarts=3",
	}, "\n")
	runner := &fakeRunner{result: command.Result{Stdout: output}}
	obs := Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Services: []string{"ssh.service"}, Runner: runner})
	if len(obs) != 1 || obs[0].Fields["exec_main_code"] != "7" || obs[0].Fields["exec_main_status"] != "9" || obs[0].Fields["restart_count"] != "3" {
		t.Fatalf("obs = %+v", obs)
	}
}

func TestCollectUnsupported(t *testing.T) {
	obs := Collect(context.Background(), Options{PlatformSupported: platform.Bool(false)})
	if len(obs) != 1 || obs[0].Supported {
		t.Fatalf("unsupported obs = %+v", obs)
	}
}
