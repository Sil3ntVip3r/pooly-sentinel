package systemd

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

type Options struct {
	SystemctlPath     string
	Services          []string
	Timeout           time.Duration
	MaxStdout         int64
	MaxStderr         int64
	PlatformSupported *bool
	Runner            Runner
}

func DefaultOptions() Options {
	return Options{
		SystemctlPath:     "/bin/systemctl",
		Services:          []string{},
		Timeout:           3 * time.Second,
		MaxStdout:         64 * 1024,
		MaxStderr:         8 * 1024,
		PlatformSupported: nil,
		Runner:            RunnerFunc(command.Run),
	}
}

func Collect(ctx context.Context, opts Options) []resources.Observation {
	opts = optionsWithDefaults(opts)
	started := time.Now()
	if ctx == nil {
		return []resources.Observation{failureObservation("systemd", "all", started, resources.ErrorInternal, "context is nil")}
	}
	if err := ctx.Err(); err != nil {
		return []resources.Observation{failureObservation("systemd", "all", started, resources.ErrorTimeout, err.Error())}
	}
	if !platform.Supported(opts.PlatformSupported) {
		return []resources.Observation{unsupportedObservation("systemd", "all")}
	}
	if len(opts.Services) == 0 {
		return []resources.Observation{successObservation("systemd", "all", started, nil, "no systemd units configured")}
	}
	observations := make([]resources.Observation, 0, len(opts.Services))
	for _, unit := range opts.Services {
		observations = append(observations, collectUnit(ctx, opts, unit))
	}
	return observations
}

func collectUnit(ctx context.Context, opts Options, unit string) resources.Observation {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return failureObservation("systemd", unit, started, resources.ErrorTimeout, err.Error())
	}
	result, err := opts.Runner.Run(ctx, command.CommandSpec{
		Path:         opts.SystemctlPath,
		Args:         []string{"show", unit, "--no-pager", "--property=" + propertyList()},
		Timeout:      opts.Timeout,
		MaxStdout:    opts.MaxStdout,
		MaxStderr:    opts.MaxStderr,
		RedactOutput: true,
	})
	state, parseErr := ParseShowOutput(unit, result.Stdout)
	if err != nil {
		class := classifyCommandError(err)
		if parseErr != nil && class == resources.ErrorCommand {
			return failureObservation("systemd", unit, started, resources.ErrorParse, parseErr.Error())
		}
		if class == resources.ErrorCommand && parseErr == nil && state.LoadState == "not-found" {
			return unitObservation(unit, state, started)
		}
		return failureObservation("systemd", unit, started, class, "systemctl show failed")
	}
	if parseErr != nil {
		return failureObservation("systemd", unit, started, resources.ErrorParse, parseErr.Error())
	}
	return unitObservation(unit, state, started)
}

func unitObservation(unit string, state UnitState, started time.Time) resources.Observation {
	metrics, err := stateMetrics(state, started.UTC())
	if err != nil {
		return failureObservation("systemd", unit, started, resources.ErrorInternal, err.Error())
	}
	obs := successObservation("systemd", unit, started, metrics, summaryForState(state))
	obs.Fields = map[string]string{
		"unit":             redaction.Redact(state.Name),
		"load_state":       state.LoadState,
		"active_state":     state.ActiveState,
		"sub_state":        state.SubState,
		"unit_file_state":  state.UnitFileState,
		"result":           state.Result,
		"main_pid":         strconv.FormatInt(state.MainPID, 10),
		"exec_main_code":   strconv.FormatInt(state.ExecMainCode, 10),
		"exec_main_status": strconv.FormatInt(state.ExecMainStatus, 10),
		"restart_count":    strconv.FormatInt(state.NRestarts, 10),
	}
	return obs
}

func stateMetrics(state UnitState, ts time.Time) ([]resources.Metric, error) {
	labels := map[string]string{"unit": normalizeUnitName(state.Name)}
	var metrics []resources.Metric
	items := []struct {
		name  string
		value float64
		unit  string
	}{
		{"pooly_systemd_unit_present", boolFloat(state.LoadState != "not-found"), "state"},
		{"pooly_systemd_unit_active", boolFloat(state.ActiveState == "active"), "state"},
		{"pooly_systemd_unit_failed", boolFloat(state.ActiveState == "failed" || state.Result != "" && state.Result != "success"), "state"},
		{"pooly_systemd_unit_activating", boolFloat(state.ActiveState == "activating"), "state"},
		{"pooly_systemd_unit_deactivating", boolFloat(state.ActiveState == "deactivating"), "state"},
		{"pooly_systemd_unit_restart_count", float64(state.NRestarts), "count"},
		{"pooly_systemd_unit_main_pid", float64(state.MainPID), "pid"},
		{"pooly_systemd_unit_exec_main_code", float64(state.ExecMainCode), "code"},
		{"pooly_systemd_unit_exec_main_status", float64(state.ExecMainStatus), "status"},
		{"pooly_systemd_unit_active_enter_monotonic_seconds", float64(state.ActiveEnterTimestampMonotonic) / 1_000_000, "seconds"},
	}
	for _, item := range items {
		metric, err := resources.NewMetric(item.name, item.value, resources.MetricGauge, item.unit, labels, ts)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func optionsWithDefaults(opts Options) Options {
	defaults := DefaultOptions()
	if opts.SystemctlPath == "" {
		opts.SystemctlPath = defaults.SystemctlPath
	}
	if opts.Timeout == 0 {
		opts.Timeout = defaults.Timeout
	}
	if opts.MaxStdout == 0 {
		opts.MaxStdout = defaults.MaxStdout
	}
	if opts.MaxStderr == 0 {
		opts.MaxStderr = defaults.MaxStderr
	}
	if opts.Runner == nil {
		opts.Runner = defaults.Runner
	}
	return opts
}

func summaryForState(state UnitState) string {
	switch {
	case state.LoadState == "not-found":
		return "unit missing"
	case state.ActiveState == "active":
		return "unit active"
	case state.ActiveState == "inactive":
		return "unit inactive"
	case state.ActiveState == "failed":
		return "unit failed state observed"
	case state.ActiveState == "activating":
		return "unit activating"
	case state.ActiveState == "deactivating":
		return "unit deactivating"
	default:
		return "unit state collected"
	}
}

func classifyCommandError(err error) resources.ErrorClass {
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
			if containsPermissionDenied(cmdErr.Error()) {
				return resources.ErrorPermissionDenied
			}
			return resources.ErrorCommand
		}
	}
	return resources.ErrorCommand
}

func containsPermissionDenied(value string) bool {
	return strings.Contains(strings.ToLower(redaction.Redact(value)), "permission denied")
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
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
		Summary:   summary,
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
