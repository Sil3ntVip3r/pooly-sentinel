package resources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type LoadAvg struct {
	Load1      float64
	Load5      float64
	Load15     float64
	Runnable   uint64
	TotalTasks uint64
	RecentPID  uint64
}

func ParseLoadAvg(data []byte) (LoadAvg, error) {
	fields := strings.Fields(string(data))
	if len(fields) < 5 {
		return LoadAvg{}, fmt.Errorf("loadavg has %d fields, want at least 5", len(fields))
	}
	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return LoadAvg{}, fmt.Errorf("load1 is malformed")
	}
	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return LoadAvg{}, fmt.Errorf("load5 is malformed")
	}
	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return LoadAvg{}, fmt.Errorf("load15 is malformed")
	}
	taskParts := strings.Split(fields[3], "/")
	if len(taskParts) != 2 {
		return LoadAvg{}, fmt.Errorf("task field is malformed")
	}
	runnable, err := strconv.ParseUint(taskParts[0], 10, 64)
	if err != nil {
		return LoadAvg{}, fmt.Errorf("runnable tasks is malformed")
	}
	total, err := strconv.ParseUint(taskParts[1], 10, 64)
	if err != nil {
		return LoadAvg{}, fmt.Errorf("total tasks is malformed")
	}
	pid, err := strconv.ParseUint(fields[4], 10, 64)
	if err != nil {
		return LoadAvg{}, fmt.Errorf("recent pid is malformed")
	}
	return LoadAvg{Load1: load1, Load5: load5, Load15: load15, Runnable: runnable, TotalTasks: total, RecentPID: pid}, nil
}

func CollectLoad(ctx context.Context, opts Options) Observation {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return failureObservation("load", "all", started, ErrorTimeout, err.Error())
	}
	data, err := opts.Source.ReadFile("/proc/loadavg")
	if err != nil {
		return failureObservation("load", "all", started, classifyReadError(err), "read /proc/loadavg failed")
	}
	load, err := ParseLoadAvg(data)
	if err != nil {
		return failureObservation("load", "all", started, ErrorParse, err.Error())
	}
	cpuCount := 0
	if statData, err := opts.Source.ReadFile("/proc/stat"); err == nil {
		if stats, err := ParseProcStatCPU(statData); err == nil {
			cpuCount = CPUCount(stats)
		}
	}
	if cpuCount <= 0 {
		cpuCount = 1
	}
	ts := started.UTC()
	var metrics []Metric
	for _, item := range []struct {
		name  string
		value float64
		unit  string
	}{
		{"pooly_cpu_load1", load.Load1, "load"},
		{"pooly_cpu_load5", load.Load5, "load"},
		{"pooly_cpu_load15", load.Load15, "load"},
		{"pooly_cpu_load1_per_cpu", load.Load1 / float64(cpuCount), "load_per_cpu"},
		{"pooly_cpu_load5_per_cpu", load.Load5 / float64(cpuCount), "load_per_cpu"},
		{"pooly_cpu_load15_per_cpu", load.Load15 / float64(cpuCount), "load_per_cpu"},
		{"pooly_tasks_runnable", float64(load.Runnable), "count"},
		{"pooly_tasks_total", float64(load.TotalTasks), "count"},
	} {
		if err := mustMetric(&metrics, item.name, item.value, MetricGauge, item.unit, nil, ts); err != nil {
			return failureObservation("load", "all", started, ErrorInternal, err.Error())
		}
	}
	return successObservation("load", "all", started, metrics, "load averages collected")
}
