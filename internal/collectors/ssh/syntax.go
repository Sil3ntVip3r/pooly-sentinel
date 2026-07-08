package ssh

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
	SSHDPath          string
	SSPath            string
	Expected          ExpectedConfig
	Timeout           time.Duration
	MaxStdout         int64
	MaxStderr         int64
	PlatformSupported *bool
	Runner            Runner
}

func DefaultOptions() Options {
	return Options{
		SSHDPath:          "/usr/sbin/sshd",
		SSPath:            "/usr/sbin/ss",
		Timeout:           3 * time.Second,
		MaxStdout:         128 * 1024,
		MaxStderr:         8 * 1024,
		PlatformSupported: nil,
		Runner:            RunnerFunc(command.Run),
	}
}

func Collect(ctx context.Context, opts Options) []resources.Observation {
	opts = optionsWithDefaults(opts)
	started := time.Now()
	if ctx == nil {
		return []resources.Observation{failureObservation("ssh", "all", started, resources.ErrorInternal, "context is nil")}
	}
	if err := ctx.Err(); err != nil {
		return []resources.Observation{failureObservation("ssh", "all", started, resources.ErrorTimeout, err.Error())}
	}
	if !platform.Supported(opts.PlatformSupported) {
		return []resources.Observation{
			unsupportedObservation("ssh_effective_config", "sshd"),
			unsupportedObservation("ssh_listeners", "tcp"),
		}
	}
	return []resources.Observation{
		collectEffective(ctx, opts),
		collectPorts(ctx, opts),
	}
}

func collectEffective(ctx context.Context, opts Options) resources.Observation {
	started := time.Now()
	expected := expectedDirectiveMap(opts.Expected)
	var metrics []resources.Metric
	matches := 0
	fields := map[string]string{
		"profiles": "poolyadmin,admin2,root",
	}
	for _, profile := range effectiveProfiles {
		result, err := opts.Runner.Run(ctx, command.CommandSpec{
			Path:         opts.SSHDPath,
			Args:         effectiveContextArgs(profile, opts.Expected),
			Timeout:      opts.Timeout,
			MaxStdout:    opts.MaxStdout,
			MaxStderr:    opts.MaxStderr,
			RedactOutput: true,
		})
		if err != nil {
			return failureObservation("ssh_effective_config", "sshd", started, effectiveCommandClass(err), "sshd effective config command failed for profile "+profile.Label)
		}
		actual, err := ParseEffectiveConfig(result.Stdout)
		if err != nil {
			return failureObservation("ssh_effective_config", "sshd", started, resources.ErrorParse, "sshd effective config output malformed for profile "+profile.Label+": "+err.Error())
		}
		for _, directive := range expectedDirectives {
			want := expected[directive]
			got := actual[directive]
			match := want != "" && got == want
			if match {
				matches++
			}
			metric, err := resources.NewMetric("pooly_ssh_directive_expected_match", boolFloat(match), resources.MetricGauge, "state", map[string]string{"directive": directive, "profile": profile.Label}, started.UTC())
			if err != nil {
				return failureObservation("ssh_effective_config", "sshd", started, resources.ErrorInternal, err.Error())
			}
			metrics = append(metrics, metric)
			fieldPrefix := profile.Label + "_" + directive
			fields[fieldPrefix+"_expected"] = want
			if got == "" {
				fields[fieldPrefix+"_actual"] = "missing"
			} else {
				fields[fieldPrefix+"_actual"] = got
			}
		}
	}
	obs := successObservation("ssh_effective_config", "sshd", started, metrics, "sshd effective config collected")
	obs.Fields = fields
	obs.Fields["matched_directives"] = strconv.Itoa(matches)
	return obs
}

func collectPorts(ctx context.Context, opts Options) resources.Observation {
	started := time.Now()
	result, err := opts.Runner.Run(ctx, command.CommandSpec{
		Path:         opts.SSPath,
		Args:         []string{"-H", "-l", "-n", "-t"},
		Timeout:      opts.Timeout,
		MaxStdout:    opts.MaxStdout,
		MaxStderr:    opts.MaxStderr,
		RedactOutput: true,
	})
	if err != nil {
		return failureObservation("ssh_listeners", "tcp", started, commandClass(err), "ss listener collection failed")
	}
	ports, err := ParseListeningPorts(result.Stdout)
	if err != nil {
		return failureObservation("ssh_listeners", "tcp", started, resources.ErrorParse, err.Error())
	}
	var metrics []resources.Metric
	for _, port := range opts.Expected.Ports {
		metric, err := resources.NewMetric("pooly_ssh_expected_port_listening", boolFloat(ports[port]), resources.MetricGauge, "state", map[string]string{"port": strconv.Itoa(port)}, started.UTC())
		if err != nil {
			return failureObservation("ssh_listeners", "tcp", started, resources.ErrorInternal, err.Error())
		}
		metrics = append(metrics, metric)
	}
	for _, port := range opts.Expected.ForbiddenPorts {
		metric, err := resources.NewMetric("pooly_ssh_forbidden_port_listening", boolFloat(ports[port]), resources.MetricGauge, "state", map[string]string{"port": strconv.Itoa(port)}, started.UTC())
		if err != nil {
			return failureObservation("ssh_listeners", "tcp", started, resources.ErrorInternal, err.Error())
		}
		metrics = append(metrics, metric)
	}
	return successObservation("ssh_listeners", "tcp", started, metrics, "ssh listening ports collected")
}

func optionsWithDefaults(opts Options) Options {
	defaults := DefaultOptions()
	if opts.SSHDPath == "" {
		opts.SSHDPath = defaults.SSHDPath
	}
	if opts.SSPath == "" {
		opts.SSPath = defaults.SSPath
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
			if containsPermissionDenied(cmdErr.Error()) {
				return resources.ErrorPermissionDenied
			}
			return resources.ErrorCommand
		}
	}
	return resources.ErrorCommand
}

func effectiveCommandClass(err error) resources.ErrorClass {
	if err == nil {
		return resources.ErrorNone
	}
	var cmdErr *command.CommandError
	if errors.As(err, &cmdErr) && cmdErr.Class == command.ErrorClassNonZeroExit {
		if containsPermissionDenied(cmdErr.Error()) {
			return resources.ErrorPermissionDenied
		}
		return resources.ErrorParse
	}
	return commandClass(err)
}

func containsPermissionDenied(value string) bool {
	return strings.Contains(strings.ToLower(redaction.Redact(value)), "permission denied")
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
