package ssh

import (
	"context"
	"errors"
	"testing"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/platform"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/command"
)

type fakeRunner struct {
	results []command.Result
	errs    []error
	specs   []command.CommandSpec
}

func (f *fakeRunner) Run(ctx context.Context, spec command.CommandSpec) (command.Result, error) {
	f.specs = append(f.specs, spec)
	result := command.Result{}
	if len(f.results) > 0 {
		result = f.results[0]
		f.results = f.results[1:]
	}
	var err error
	if len(f.errs) > 0 {
		err = f.errs[0]
		f.errs = f.errs[1:]
	}
	return result, err
}

func expectedFixture() ExpectedConfig {
	return ExpectedConfig{
		Ports:                        []int{6200},
		ForbiddenPorts:               []int{22},
		PermitRootLogin:              "no",
		PasswordAuthentication:       "no",
		KbdInteractiveAuthentication: "no",
		PermitEmptyPasswords:         "no",
		PubkeyAuthentication:         "yes",
		StrictModes:                  "yes",
		PermitUserEnvironment:        "no",
	}
}

func TestParseEffectiveConfig(t *testing.T) {
	values, err := ParseEffectiveConfig("PermitRootLogin no\nPasswordAuthentication yes\nPasswordAuthentication no\nPermitRootLogin yes\n")
	if err != nil {
		t.Fatal(err)
	}
	if values["permitrootlogin"] != "yes" || values["passwordauthentication"] != "no" {
		t.Fatalf("values = %+v", values)
	}
	if _, err := ParseEffectiveConfig("broken\n"); err == nil {
		t.Fatal("malformed effective config error = nil")
	}
}

func TestParseListeningPorts(t *testing.T) {
	ports, err := ParseListeningPorts("LISTEN 0 128 0.0.0.0:22 0.0.0.0:*\nLISTEN 0 128 [::]:6200 [::]:*\nLISTEN 0 128 *:2222 *:*\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{22, 6200, 2222} {
		if !ports[port] {
			t.Fatalf("missing port %d in %+v", port, ports)
		}
	}
	if _, err := ParseListeningPorts("LISTEN too short\n"); err == nil {
		t.Fatal("malformed listener error = nil")
	}
}

func TestCollectSSHConfigAndPorts(t *testing.T) {
	runner := &fakeRunner{results: []command.Result{
		{Stdout: "permitrootlogin no\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitemptypasswords no\npubkeyauthentication yes\nstrictmodes yes\npermituserenvironment no\n"},
		{Stdout: "LISTEN 0 128 0.0.0.0:22 0.0.0.0:*\nLISTEN 0 128 [::]:6200 [::]:*\n"},
	}}
	obs := Collect(context.Background(), Options{
		PlatformSupported: platform.Bool(true),
		Expected:          expectedFixture(),
		Runner:            runner,
	})
	if len(obs) != 2 || !obs[0].Success || !obs[1].Success {
		t.Fatalf("obs = %+v", obs)
	}
	if got := obs[0].Fields["passwordauthentication_actual"]; got != "yes" {
		t.Fatalf("actual passwordauthentication = %q", got)
	}
	if runner.specs[0].Args[0] != "-T" || runner.specs[1].Args[0] != "-H" {
		t.Fatalf("unexpected command args: %#v", runner.specs)
	}
}

func TestCollectSSHTimeoutAndUnsupported(t *testing.T) {
	runner := &fakeRunner{
		results: []command.Result{{Stdout: "permitrootlogin no\n"}},
		errs:    []error{&command.CommandError{Class: command.ErrorClassTimeout, Err: errors.New("deadline")}},
	}
	obs := Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Expected: expectedFixture(), Runner: runner})
	if len(obs) != 2 || obs[0].ErrorClass != resources.ErrorTimeout {
		t.Fatalf("timeout obs = %+v", obs)
	}
	obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(false)})
	if len(obs) != 2 || obs[0].Supported || obs[1].Supported {
		t.Fatalf("unsupported obs = %+v", obs)
	}
}

func TestCollectSSHCommandErrorClasses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want resources.ErrorClass
	}{
		{name: "missing executable", err: &command.CommandError{Class: command.ErrorClassMissingExecutable, Err: errors.New("missing")}, want: resources.ErrorSourceMissing},
		{name: "syntax failure", err: &command.CommandError{Class: command.ErrorClassNonZeroExit, Err: errors.New("bad config")}, want: resources.ErrorParse},
		{name: "permission", err: &command.CommandError{Class: command.ErrorClassNonZeroExit, Stderr: "Permission denied", Err: errors.New("permission")}, want: resources.ErrorPermissionDenied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{errs: []error{tc.err}}
			obs := Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Expected: expectedFixture(), Runner: runner})
			if len(obs) != 2 || obs[0].Success || obs[0].ErrorClass != tc.want {
				t.Fatalf("obs = %+v, want %s", obs, tc.want)
			}
		})
	}
}
