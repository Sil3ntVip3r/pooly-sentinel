package journal

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/platform"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/command"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
)

type Runner interface {
	Run(ctx context.Context, spec command.CommandSpec) (command.Result, error)
}

type RunnerFunc func(ctx context.Context, spec command.CommandSpec) (command.Result, error)

func (f RunnerFunc) Run(ctx context.Context, spec command.CommandSpec) (command.Result, error) {
	return f(ctx, spec)
}

type StreamConfig struct {
	Name          string
	Enabled       bool
	Timeout       time.Duration
	MaxRecords    int
	MaxBytes      int64
	MaxFieldBytes int
}

type Options struct {
	JournalctlPath    string
	Streams           []StreamConfig
	State             resources.StateStore
	Persist           bool
	PlatformSupported *bool
	Runner            Runner
}

func DefaultOptions() Options {
	return Options{
		JournalctlPath:    "/bin/journalctl",
		PlatformSupported: nil,
		Runner:            RunnerFunc(command.Run),
		Streams: []StreamConfig{
			{Name: "auth", Enabled: true, Timeout: 3 * time.Second, MaxRecords: 100, MaxBytes: 256 * 1024, MaxFieldBytes: 512},
			{Name: "services", Enabled: true, Timeout: 3 * time.Second, MaxRecords: 100, MaxBytes: 256 * 1024, MaxFieldBytes: 512},
			{Name: "kernel", Enabled: true, Timeout: 3 * time.Second, MaxRecords: 100, MaxBytes: 256 * 1024, MaxFieldBytes: 512},
		},
	}
}

func Collect(ctx context.Context, opts Options) []resources.Observation {
	opts = optionsWithDefaults(opts)
	started := time.Now()
	if ctx == nil {
		return []resources.Observation{failureObservation("journal", "all", started, resources.ErrorInternal, "context is nil")}
	}
	if err := ctx.Err(); err != nil {
		return []resources.Observation{failureObservation("journal", "all", started, resources.ErrorTimeout, err.Error())}
	}
	if !platform.Supported(opts.PlatformSupported) {
		streams := enabledStreams(opts.Streams)
		if len(streams) == 0 {
			return []resources.Observation{unsupportedObservation("journal", "all")}
		}
		observations := make([]resources.Observation, 0, len(streams))
		for _, stream := range streams {
			observations = append(observations, unsupportedObservation("journal", stream.Name))
		}
		return observations
	}
	streams := enabledStreams(opts.Streams)
	if len(streams) == 0 {
		return []resources.Observation{successObservation("journal", "all", started, nil, "no journal streams enabled")}
	}
	observations := make([]resources.Observation, 0, len(streams))
	for _, stream := range streams {
		observations = append(observations, collectStream(ctx, opts, stream))
	}
	return observations
}

func collectStream(ctx context.Context, opts Options, stream StreamConfig) resources.Observation {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return failureObservation("journal", stream.Name, started, resources.ErrorTimeout, err.Error())
	}
	cursor, hasCursor, err := loadCursor(ctx, opts.State, stream.Name)
	if err != nil {
		if hasCursor {
			return baselineAfterReset(ctx, opts, stream, started, "journal cursor state reset")
		}
		return failureObservation("journal", stream.Name, started, resources.ErrorState, "load journal cursor failed")
	}
	if opts.Persist && !hasCursor {
		return establishBaseline(ctx, opts, stream, started, false)
	}
	result, err := opts.Runner.Run(ctx, command.CommandSpec{
		Path:         opts.JournalctlPath,
		Args:         journalArgs(stream, cursor, stream.MaxRecords),
		Timeout:      stream.Timeout,
		MaxStdout:    stream.MaxBytes,
		MaxStderr:    8 * 1024,
		RedactOutput: true,
	})
	if err != nil {
		if hasCursor && isCursorInvalidation(err) {
			return baselineAfterReset(ctx, opts, stream, started, "journal cursor reset")
		}
		return failureObservation("journal", stream.Name, started, commandClass(err), "journalctl collection failed")
	}
	records, lastCursor, truncated, err := ParseJSONLines([]byte(result.Stdout), ParseOptions{
		Stream:        stream.Name,
		MaxRecords:    stream.MaxRecords,
		MaxFieldBytes: stream.MaxFieldBytes,
	})
	if err != nil {
		return failureObservation("journal", stream.Name, started, resources.ErrorParse, err.Error())
	}
	if truncated {
		obs := streamObservation(stream.Name, started, records, truncated, "journal output truncated")
		obs.Success = false
		obs.Stale = true
		obs.ErrorClass = resources.ErrorParse
		return obs
	}
	if err := ctx.Err(); err != nil {
		return failureObservation("journal", stream.Name, started, resources.ErrorTimeout, err.Error())
	}
	if err := saveCursor(ctx, opts.State, opts.Persist, stream.Name, lastCursor); err != nil {
		return failureObservation("journal", stream.Name, started, resources.ErrorState, "save journal cursor failed")
	}
	return streamObservation(stream.Name, started, records, truncated, "journal records collected")
}

func establishBaseline(ctx context.Context, opts Options, stream StreamConfig, started time.Time, stale bool) resources.Observation {
	result, err := opts.Runner.Run(ctx, command.CommandSpec{
		Path:         opts.JournalctlPath,
		Args:         journalArgs(stream, "", 1),
		Timeout:      stream.Timeout,
		MaxStdout:    stream.MaxBytes,
		MaxStderr:    8 * 1024,
		RedactOutput: true,
	})
	if err != nil {
		return failureObservation("journal", stream.Name, started, commandClass(err), "journalctl baseline failed")
	}
	_, lastCursor, truncated, err := ParseJSONLines([]byte(result.Stdout), ParseOptions{
		Stream:        stream.Name,
		MaxRecords:    1,
		MaxFieldBytes: stream.MaxFieldBytes,
	})
	if err != nil {
		return failureObservation("journal", stream.Name, started, resources.ErrorParse, err.Error())
	}
	if truncated {
		return failureObservation("journal", stream.Name, started, resources.ErrorParse, "journal baseline truncated")
	}
	if err := ctx.Err(); err != nil {
		return failureObservation("journal", stream.Name, started, resources.ErrorTimeout, err.Error())
	}
	if err := saveCursor(ctx, opts.State, opts.Persist, stream.Name, lastCursor); err != nil {
		return failureObservation("journal", stream.Name, started, resources.ErrorState, "save journal cursor failed")
	}
	obs := successObservation("journal", stream.Name, started, nil, "journal cursor baseline recorded")
	obs.Stale = stale
	if stale {
		obs.ErrorClass = resources.ErrorCounterReset
	}
	return obs
}

func baselineAfterReset(ctx context.Context, opts Options, stream StreamConfig, started time.Time, summary string) resources.Observation {
	obs := establishBaseline(ctx, opts, stream, started, true)
	if obs.Success {
		obs.Summary = summary
		obs.Stale = true
		obs.ErrorClass = resources.ErrorCounterReset
	}
	return obs
}

func streamObservation(stream string, started time.Time, records []Record, truncated bool, summary string) resources.Observation {
	ts := started.UTC()
	events := make([]resources.Event, 0, len(records))
	for _, record := range records {
		events = append(events, eventFromRecord(stream, record))
	}
	metrics := make([]resources.Metric, 0, 2)
	countMetric, err := resources.NewMetric("pooly_journal_events_total", float64(len(records)), resources.MetricGauge, "count", map[string]string{"stream": stream}, ts)
	if err == nil {
		metrics = append(metrics, countMetric)
	}
	truncatedMetric, err := resources.NewMetric("pooly_journal_truncated", boolFloat(truncated), resources.MetricGauge, "state", map[string]string{"stream": stream}, ts)
	if err == nil {
		metrics = append(metrics, truncatedMetric)
	}
	obs := successObservation("journal", stream, started, metrics, summary)
	obs.Events = events
	obs.Fields = map[string]string{
		"stream":      stream,
		"event_count": strconv.Itoa(len(records)),
		"truncated":   strconv.FormatBool(truncated),
	}
	return obs
}

func journalArgs(stream StreamConfig, cursor string, lines int) []string {
	args := []string{"--no-pager", "--output=json"}
	if cursor != "" {
		args = append(args, "--after-cursor", cursor)
	} else {
		args = append(args, "--lines="+strconv.Itoa(lines))
	}
	switch stream.Name {
	case "auth":
		args = append(args, "--facility=auth,authpriv")
	case "kernel":
		args = append(args, "-k")
	case "services":
		args = append(args, "_SYSTEMD_UNIT=*.service")
	}
	return args
}

func enabledStreams(streams []StreamConfig) []StreamConfig {
	var out []StreamConfig
	for _, stream := range streams {
		if stream.Enabled {
			out = append(out, stream)
		}
	}
	return out
}

func optionsWithDefaults(opts Options) Options {
	defaults := DefaultOptions()
	if opts.JournalctlPath == "" {
		opts.JournalctlPath = defaults.JournalctlPath
	}
	if opts.Runner == nil {
		opts.Runner = defaults.Runner
	}
	if len(opts.Streams) == 0 {
		opts.Streams = defaults.Streams
	}
	for i := range opts.Streams {
		if opts.Streams[i].Timeout == 0 {
			opts.Streams[i].Timeout = 3 * time.Second
		}
		if opts.Streams[i].MaxRecords <= 0 {
			opts.Streams[i].MaxRecords = 100
		}
		if opts.Streams[i].MaxBytes <= 0 {
			opts.Streams[i].MaxBytes = 256 * 1024
		}
		if opts.Streams[i].MaxFieldBytes <= 0 {
			opts.Streams[i].MaxFieldBytes = 512
		}
	}
	return opts
}

func commandClass(err error) resources.ErrorClass {
	if err == nil {
		return resources.ErrorNone
	}
	var cmdErr *command.CommandError
	if errors.As(err, &cmdErr) {
		switch cmdErr.Class {
		case command.ErrorClassTimeout, command.ErrorClassCanceled:
			return resources.ErrorTimeout
		case command.ErrorClassMissingExecutable:
			return resources.ErrorSourceMissing
		case command.ErrorClassOutputLimit:
			return resources.ErrorParse
		default:
			if strings.Contains(strings.ToLower(redaction.Redact(cmdErr.Stderr)), "permission denied") {
				return resources.ErrorPermissionDenied
			}
			return resources.ErrorCommand
		}
	}
	return resources.ErrorCommand
}

func isCursorInvalidation(err error) bool {
	var cmdErr *command.CommandError
	if !errors.As(err, &cmdErr) {
		return false
	}
	if cmdErr.Class != command.ErrorClassNonZeroExit && cmdErr.Class != command.ErrorClassStartFailed {
		return false
	}
	text := strings.ToLower(redaction.Redact(cmdErr.Error() + " " + cmdErr.Stderr))
	return strings.Contains(text, "cursor") &&
		(strings.Contains(text, "invalid") || strings.Contains(text, "failed to seek") || strings.Contains(text, "cannot seek"))
}

func successObservation(name string, target string, started time.Time, metrics []resources.Metric, summary string) resources.Observation {
	return resources.Observation{
		Collector: name,
		Target:    target,
		Timestamp: started.UTC(),
		Duration:  time.Since(started),
		Success:   true,
		Supported: true,
		Metrics:   metrics,
		Summary:   redaction.Redact(summary),
	}
}

func failureObservation(name string, target string, started time.Time, class resources.ErrorClass, summary string) resources.Observation {
	return resources.Observation{
		Collector:  name,
		Target:     target,
		Timestamp:  started.UTC(),
		Duration:   time.Since(started),
		Success:    false,
		Supported:  class != resources.ErrorUnsupported,
		Summary:    redaction.Redact(summary),
		ErrorClass: class,
	}
}

func unsupportedObservation(name string, target string) resources.Observation {
	started := time.Now()
	return failureObservation(name, target, started, resources.ErrorUnsupported, "unsupported platform")
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
