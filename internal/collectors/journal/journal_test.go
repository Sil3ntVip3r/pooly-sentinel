package journal

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
	results []command.Result
	errs    []error
	specs   []command.CommandSpec
}

func (f *fakeRunner) Run(ctx context.Context, spec command.CommandSpec) (command.Result, error) {
	f.specs = append(f.specs, spec)
	if len(f.results) == 0 {
		return command.Result{}, nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	var err error
	if len(f.errs) > 0 {
		err = f.errs[0]
		f.errs = f.errs[1:]
	}
	return result, err
}

func TestParseJSONLinesNormalizesAndRedacts(t *testing.T) {
	data := []byte(`{"__CURSOR":"c1","__REALTIME_TIMESTAMP":"1710000000000000","PRIORITY":"5","_TRANSPORT":"syslog","_SYSTEMD_UNIT":"ssh.service","_COMM":"sshd","MESSAGE":"Failed password for token=supersecret"}` + "\n")
	records, cursor, truncated, err := ParseJSONLines(data, ParseOptions{Stream: "auth", MaxRecords: 10, MaxFieldBytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "c1" || truncated || len(records) != 1 {
		t.Fatalf("records=%+v cursor=%q truncated=%t", records, cursor, truncated)
	}
	if records[0].Category != "authentication_failure" {
		t.Fatalf("category = %q", records[0].Category)
	}
	event := eventFromRecord("auth", records[0])
	if strings.Contains(event.Summary, "supersecret") {
		t.Fatalf("event leaked secret: %+v", event)
	}
}

func TestParseJSONLinesMalformedAndBounded(t *testing.T) {
	if _, _, _, err := ParseJSONLines([]byte("{bad}\n"), ParseOptions{Stream: "auth"}); err == nil {
		t.Fatal("malformed JSON error = nil")
	}
	data := []byte(`{"__CURSOR":"c1","MESSAGE":"one"}` + "\n" + `{"__CURSOR":"c2","MESSAGE":"two"}` + "\n")
	records, cursor, truncated, err := ParseJSONLines(data, ParseOptions{Stream: "kernel", MaxRecords: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || cursor != "c1" || !truncated {
		t.Fatalf("bounded parse = records=%d cursor=%q truncated=%t", len(records), cursor, truncated)
	}
}

func TestCollectCursorPersistenceDryRunAndReset(t *testing.T) {
	store := resources.NewMemoryStateStore()
	runner := &fakeRunner{results: []command.Result{{Stdout: `{"__CURSOR":"baseline","MESSAGE":"Started ssh.service"}` + "\n"}}}
	obs := Collect(context.Background(), Options{
		PlatformSupported: platform.Bool(true),
		Persist:           true,
		State:             store,
		Runner:            runner,
		Streams:           []StreamConfig{{Name: "services", Enabled: true, MaxRecords: 10, MaxBytes: 4096, MaxFieldBytes: 128}},
	})
	if len(obs) != 1 || !obs[0].Success || obs[0].Summary != "journal cursor baseline recorded" {
		t.Fatalf("baseline obs = %+v", obs)
	}
	if _, _, ok := strings.Cut(strings.Join(runner.specs[0].Args, " "), "--after-cursor"); ok {
		t.Fatalf("baseline unexpectedly used cursor args: %#v", runner.specs[0].Args)
	}

	runner = &fakeRunner{results: []command.Result{{Stdout: `{"__CURSOR":"next","MESSAGE":"Failed ssh.service","_SYSTEMD_UNIT":"ssh.service"}` + "\n"}}}
	obs = Collect(context.Background(), Options{
		PlatformSupported: platform.Bool(true),
		Persist:           false,
		State:             store,
		Runner:            runner,
		Streams:           []StreamConfig{{Name: "services", Enabled: true, MaxRecords: 10, MaxBytes: 4096, MaxFieldBytes: 128}},
	})
	if len(obs) != 1 || len(obs[0].Events) != 1 || !strings.Contains(strings.Join(runner.specs[0].Args, " "), "--after-cursor") {
		t.Fatalf("incremental dry-run obs=%+v args=%#v", obs, runner.specs)
	}
	raw, err := store.Get(context.Background(), "journal", "services")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "baseline") || strings.Contains(raw, "next") {
		t.Fatalf("dry-run changed cursor state: %s", raw)
	}

	badStore := resources.NewMemoryStateStore()
	if err := badStore.Upsert(context.Background(), "journal", "auth", "ok", "{bad"); err != nil {
		t.Fatal(err)
	}
	runner = &fakeRunner{results: []command.Result{{Stdout: `{"__CURSOR":"fresh","MESSAGE":"Accepted publickey"}` + "\n"}}}
	obs = Collect(context.Background(), Options{
		PlatformSupported: platform.Bool(true),
		Persist:           true,
		State:             badStore,
		Runner:            runner,
		Streams:           []StreamConfig{{Name: "auth", Enabled: true, MaxRecords: 10, MaxBytes: 4096, MaxFieldBytes: 128}},
	})
	if len(obs) != 1 || !obs[0].Stale || obs[0].ErrorClass != resources.ErrorCounterReset {
		t.Fatalf("reset obs = %+v", obs)
	}
}

func TestCollectTimeoutAndUnsupported(t *testing.T) {
	runner := &fakeRunner{
		results: []command.Result{{}},
		errs:    []error{&command.CommandError{Class: command.ErrorClassTimeout, Err: errors.New("deadline")}},
	}
	obs := Collect(context.Background(), Options{
		PlatformSupported: platform.Bool(true),
		Runner:            runner,
		Streams:           []StreamConfig{{Name: "kernel", Enabled: true, MaxBytes: 4096}},
	})
	if len(obs) != 1 || obs[0].ErrorClass != resources.ErrorTimeout {
		t.Fatalf("timeout obs = %+v", obs)
	}
	obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(false)})
	if len(obs) != 3 {
		t.Fatalf("unsupported obs = %+v", obs)
	}
	for _, observation := range obs {
		if observation.Supported {
			t.Fatalf("unsupported obs = %+v", obs)
		}
	}
}

func TestJournalDoesNotSaveCursorAfterIncompleteProcessing(t *testing.T) {
	cases := []struct {
		name      string
		result    command.Result
		err       error
		wantClass resources.ErrorClass
	}{
		{
			name:      "truncated",
			result:    command.Result{Stdout: `{"__CURSOR":"c1","MESSAGE":"one"}` + "\n" + `{"__CURSOR":"c2","MESSAGE":"two"}` + "\n"},
			wantClass: resources.ErrorParse,
		},
		{
			name:      "malformed",
			result:    command.Result{Stdout: "{bad}\n"},
			wantClass: resources.ErrorParse,
		},
		{
			name:      "output limit",
			err:       &command.CommandError{Class: command.ErrorClassOutputLimit, Err: errors.New("limit")},
			wantClass: resources.ErrorParse,
		},
		{
			name:      "timeout with partial stdout",
			result:    command.Result{Stdout: `{"__CURSOR":"c1","MESSAGE":"one"}` + "\n"},
			err:       &command.CommandError{Class: command.ErrorClassTimeout, Err: errors.New("deadline")},
			wantClass: resources.ErrorTimeout,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := resources.NewMemoryStateStore()
			if err := saveCursor(context.Background(), store, true, "auth", "old"); err != nil {
				t.Fatal(err)
			}
			runner := &fakeRunner{results: []command.Result{tc.result}, errs: []error{tc.err}}
			obs := Collect(context.Background(), Options{
				PlatformSupported: platform.Bool(true),
				Persist:           true,
				State:             store,
				Runner:            runner,
				Streams:           []StreamConfig{{Name: "auth", Enabled: true, MaxRecords: 1, MaxBytes: 4096, MaxFieldBytes: 128}},
			})
			if len(obs) != 1 || obs[0].Success || obs[0].ErrorClass != tc.wantClass {
				t.Fatalf("obs = %+v", obs)
			}
			raw, err := store.Get(context.Background(), "journal", "auth")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(raw, "old") || strings.Contains(raw, "c1") || strings.Contains(raw, "c2") {
				t.Fatalf("cursor was overwritten after %s: %s", tc.name, raw)
			}
		})
	}
}

func TestJournalCursorInvalidationResetIsNarrow(t *testing.T) {
	store := resources.NewMemoryStateStore()
	if err := saveCursor(context.Background(), store, true, "auth", "old"); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		results: []command.Result{{}, {Stdout: `{"__CURSOR":"fresh","MESSAGE":"baseline"}` + "\n"}},
		errs:    []error{&command.CommandError{Class: command.ErrorClassNonZeroExit, Stderr: "Invalid cursor", Err: errors.New("invalid cursor")}},
	}
	obs := Collect(context.Background(), Options{
		PlatformSupported: platform.Bool(true),
		Persist:           true,
		State:             store,
		Runner:            runner,
		Streams:           []StreamConfig{{Name: "auth", Enabled: true, MaxRecords: 10, MaxBytes: 4096, MaxFieldBytes: 128}},
	})
	if len(obs) != 1 || !obs[0].Stale || obs[0].ErrorClass != resources.ErrorCounterReset {
		t.Fatalf("cursor reset obs = %+v", obs)
	}
	raw, err := store.Get(context.Background(), "journal", "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "fresh") {
		t.Fatalf("cursor reset did not save fresh baseline: %s", raw)
	}
}
