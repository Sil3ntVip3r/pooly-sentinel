package resources

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
)

type MetricKind string

const (
	MetricGauge   MetricKind = "gauge"
	MetricCounter MetricKind = "counter"
)

type ErrorClass string

const (
	ErrorNone             ErrorClass = ""
	ErrorUnsupported      ErrorClass = "unsupported"
	ErrorSourceMissing    ErrorClass = "source_missing"
	ErrorPermissionDenied ErrorClass = "permission_denied"
	ErrorParse            ErrorClass = "parse_error"
	ErrorTimeout          ErrorClass = "timeout"
	ErrorCounterReset     ErrorClass = "counter_reset"
	ErrorState            ErrorClass = "state_error"
	ErrorInternal         ErrorClass = "internal_error"
)

type Metric struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Kind      MetricKind        `json:"kind"`
	Unit      string            `json:"unit"`
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

type Observation struct {
	Collector  string        `json:"collector"`
	Target     string        `json:"target"`
	Timestamp  time.Time     `json:"timestamp"`
	Duration   time.Duration `json:"duration"`
	Success    bool          `json:"success"`
	Supported  bool          `json:"supported"`
	Stale      bool          `json:"stale"`
	Metrics    []Metric      `json:"metrics,omitempty"`
	Summary    string        `json:"summary,omitempty"`
	ErrorClass ErrorClass    `json:"error_class,omitempty"`
}

type FileSource interface {
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

type OSFileSource struct{}

func (OSFileSource) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(osDirFS{}, name)
}

func (OSFileSource) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(osDirFS{}, name)
}

type osDirFS struct{}

func (osDirFS) Open(name string) (fs.File, error) {
	return openOSFile(name)
}

type StateStore interface {
	Get(ctx context.Context, collector string, target string) (string, error)
	Upsert(ctx context.Context, collector string, target string, status string, stateJSON string) error
}

type Options struct {
	Source              FileSource
	State               StateStore
	Persist             bool
	PlatformSupported   bool
	FilesystemMounts    []string
	DiskAutoDiscover    bool
	DiskExclude         []string
	NetworkAutoDiscover bool
	NetworkInclude      []string
	NetworkExclude      []string
	PressureMissingOK   bool
	CPUEnabled          bool
	LoadEnabled         bool
	MemoryEnabled       bool
	PressureEnabled     bool
	FilesystemEnabled   bool
	DiskIOEnabled       bool
	NetworkEnabled      bool
	UptimeEnabled       bool
}

type CollectorInfo struct {
	Name      string
	Enabled   bool
	Supported bool
}

var metricNamePattern = regexp.MustCompile(`^pooly_[a-z0-9_]+$`)

var allowedLabels = map[string]struct{}{
	"collector":     {},
	"cpu":           {},
	"mount":         {},
	"device":        {},
	"interface":     {},
	"pressure_type": {},
	"window":        {},
}

func NewMetric(name string, value float64, kind MetricKind, unit string, labels map[string]string, ts time.Time) (Metric, error) {
	if !metricNamePattern.MatchString(name) {
		return Metric{}, fmt.Errorf("invalid metric name %q", name)
	}
	if kind != MetricGauge && kind != MetricCounter {
		return Metric{}, fmt.Errorf("invalid metric kind %q", kind)
	}
	cleanLabels := map[string]string{}
	for key, value := range labels {
		if _, ok := allowedLabels[key]; !ok {
			return Metric{}, fmt.Errorf("label %q is not allowed", key)
		}
		if value == "" {
			return Metric{}, fmt.Errorf("label %q is empty", key)
		}
		if strings.ContainsAny(value, "\n\r\t") {
			return Metric{}, fmt.Errorf("label %q contains control characters", key)
		}
		cleanLabels[key] = value
	}
	return Metric{Name: name, Value: value, Kind: kind, Unit: unit, Labels: cleanLabels, Timestamp: ts.UTC()}, nil
}

func mustMetric(metrics *[]Metric, name string, value float64, kind MetricKind, unit string, labels map[string]string, ts time.Time) error {
	metric, err := NewMetric(name, value, kind, unit, labels, ts)
	if err != nil {
		return err
	}
	*metrics = append(*metrics, metric)
	return nil
}

func successObservation(name string, target string, started time.Time, metrics []Metric, summary string) Observation {
	return Observation{
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

func failureObservation(name string, target string, started time.Time, class ErrorClass, summary string) Observation {
	return Observation{
		Collector:  name,
		Target:     target,
		Timestamp:  started.UTC(),
		Duration:   time.Since(started),
		Success:    false,
		Supported:  class != ErrorUnsupported,
		Summary:    summary,
		ErrorClass: class,
	}
}

func unsupportedObservation(name string, target string) Observation {
	started := time.Now().UTC()
	return failureObservation(name, target, started, ErrorUnsupported, "unsupported platform")
}

func classifyReadError(err error) ErrorClass {
	if err == nil {
		return ErrorNone
	}
	if errors.Is(err, fs.ErrNotExist) {
		return ErrorSourceMissing
	}
	if errors.Is(err, fs.ErrPermission) {
		return ErrorPermissionDenied
	}
	return ErrorInternal
}

func optionsWithDefaults(opts Options) Options {
	if opts.Source == nil {
		opts.Source = OSFileSource{}
	}
	if !opts.PlatformSupported {
		opts.PlatformSupported = runtime.GOOS == "linux"
	}
	if len(opts.FilesystemMounts) == 0 {
		opts.FilesystemMounts = []string{"/", "/home", "/var", "/var/log", "/var/lib", "/var/lib/pooly-sentinel", "/var/log/pooly-sentinel"}
	}
	if len(opts.DiskExclude) == 0 {
		opts.DiskExclude = []string{"loop*", "ram*", "fd*", "sr*"}
	}
	if len(opts.NetworkExclude) == 0 {
		opts.NetworkExclude = []string{"lo", "docker*", "veth*", "br-*"}
	}
	return opts
}

func DefaultOptions() Options {
	return Options{
		Source:              OSFileSource{},
		PlatformSupported:   runtime.GOOS == "linux",
		PressureMissingOK:   true,
		CPUEnabled:          true,
		LoadEnabled:         true,
		MemoryEnabled:       true,
		PressureEnabled:     true,
		FilesystemEnabled:   true,
		DiskAutoDiscover:    true,
		DiskIOEnabled:       true,
		NetworkAutoDiscover: true,
		NetworkEnabled:      true,
		UptimeEnabled:       true,
	}
}

func ListCollectors(opts Options) []CollectorInfo {
	opts = optionsWithDefaults(opts)
	return []CollectorInfo{
		{Name: "cpu", Enabled: opts.CPUEnabled, Supported: opts.PlatformSupported},
		{Name: "load", Enabled: opts.LoadEnabled, Supported: opts.PlatformSupported},
		{Name: "memory", Enabled: opts.MemoryEnabled, Supported: opts.PlatformSupported},
		{Name: "pressure", Enabled: opts.PressureEnabled, Supported: opts.PlatformSupported},
		{Name: "filesystem", Enabled: opts.FilesystemEnabled, Supported: opts.PlatformSupported},
		{Name: "diskio", Enabled: opts.DiskIOEnabled, Supported: opts.PlatformSupported},
		{Name: "network", Enabled: opts.NetworkEnabled, Supported: opts.PlatformSupported},
		{Name: "uptime", Enabled: opts.UptimeEnabled, Supported: opts.PlatformSupported},
	}
}

func Collect(ctx context.Context, opts Options) []Observation {
	opts = optionsWithDefaults(opts)
	if ctx == nil {
		return []Observation{failureObservation("resources", "all", time.Now(), ErrorInternal, "context is nil")}
	}
	if err := ctx.Err(); err != nil {
		return []Observation{failureObservation("resources", "all", time.Now(), ErrorTimeout, err.Error())}
	}
	if !opts.PlatformSupported {
		var observations []Observation
		for _, info := range ListCollectors(opts) {
			if info.Enabled {
				observations = append(observations, unsupportedObservation(info.Name, "all"))
			}
		}
		return observations
	}
	var observations []Observation
	if opts.CPUEnabled {
		observations = append(observations, CollectCPU(ctx, opts))
	}
	if opts.LoadEnabled {
		observations = append(observations, CollectLoad(ctx, opts))
	}
	if opts.MemoryEnabled {
		observations = append(observations, CollectMemory(ctx, opts))
	}
	if opts.PressureEnabled {
		observations = append(observations, CollectPressure(ctx, opts)...)
	}
	if opts.FilesystemEnabled {
		observations = append(observations, CollectFilesystems(ctx, opts)...)
	}
	if opts.DiskIOEnabled {
		observations = append(observations, CollectDiskIO(ctx, opts)...)
	}
	if opts.NetworkEnabled {
		observations = append(observations, CollectNetwork(ctx, opts)...)
	}
	if opts.UptimeEnabled {
		observations = append(observations, CollectUptime(ctx, opts))
	}
	return observations
}

func RequiredFailed(observations []Observation) bool {
	for _, observation := range observations {
		if !observation.Success && observation.Supported {
			return true
		}
	}
	return false
}

func MetricNames(observation Observation) []string {
	names := make([]string, 0, len(observation.Metrics))
	for _, metric := range observation.Metrics {
		names = append(names, metric.Name)
	}
	sort.Strings(names)
	return names
}

func uniqueNormalized(values []string, clean func(string) (string, error)) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		normalized, err := clean(value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[normalized]; ok {
			return nil, fmt.Errorf("duplicate entry %q", normalized)
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func matchesAny(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if ok, _ := pathMatch(pattern, value); ok {
			return true
		}
	}
	return false
}

func includeName(value string, include []string, exclude []string) bool {
	if len(include) > 0 && !slices.Contains(include, value) && !matchesAny(value, include) {
		return false
	}
	return !matchesAny(value, exclude)
}
