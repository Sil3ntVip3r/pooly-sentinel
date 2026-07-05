package resources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type UptimeInfo struct {
	UptimeSeconds float64
	BootTimeUnix  uint64
	BootID        string
}

type bootState struct {
	BootID string `json:"boot_id"`
}

func ParseProcUptime(data []byte) (float64, error) {
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("uptime is truncated")
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("uptime is malformed")
	}
	if value < 0 {
		return 0, fmt.Errorf("uptime cannot be negative")
	}
	return value, nil
}

func ParseBootTime(data []byte) (uint64, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "btime" {
			continue
		}
		if len(fields) != 2 {
			return 0, fmt.Errorf("btime line is malformed")
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("btime is malformed")
		}
		return value, nil
	}
	return 0, fmt.Errorf("btime not found")
}

func ParseBootID(data []byte) (string, error) {
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("boot id is empty")
	}
	if len(value) < 8 || strings.ContainsAny(value, " \t\r\n") {
		return "", fmt.Errorf("boot id is malformed")
	}
	return value, nil
}

func CollectUptime(ctx context.Context, opts Options) Observation {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return failureObservation("uptime", "system", started, ErrorTimeout, err.Error())
	}
	uptimeData, err := opts.Source.ReadFile("/proc/uptime")
	if err != nil {
		return failureObservation("uptime", "system", started, classifyReadError(err), "read /proc/uptime failed")
	}
	statData, err := opts.Source.ReadFile("/proc/stat")
	if err != nil {
		return failureObservation("uptime", "system", started, classifyReadError(err), "read /proc/stat failed")
	}
	bootIDData, err := opts.Source.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return failureObservation("uptime", "system", started, classifyReadError(err), "read boot_id failed")
	}
	uptimeSeconds, err := ParseProcUptime(uptimeData)
	if err != nil {
		return failureObservation("uptime", "system", started, ErrorParse, err.Error())
	}
	bootTime, err := ParseBootTime(statData)
	if err != nil {
		return failureObservation("uptime", "system", started, ErrorParse, err.Error())
	}
	bootID, err := ParseBootID(bootIDData)
	if err != nil {
		return failureObservation("uptime", "system", started, ErrorParse, err.Error())
	}
	previous, ok, err := loadState[bootState](ctx, opts.State, "uptime", "boot")
	if err != nil {
		return failureObservation("uptime", "system", started, ErrorState, "load boot state failed")
	}
	changed := 0.0
	if ok && previous.BootID != "" && previous.BootID != bootID {
		changed = 1
	}
	if err := saveState(ctx, opts.State, opts.Persist, "uptime", "boot", bootState{BootID: bootID}); err != nil {
		return failureObservation("uptime", "system", started, ErrorState, "save boot state failed")
	}
	ts := started.UTC()
	var metrics []Metric
	for _, item := range []struct {
		name  string
		value float64
		unit  string
	}{
		{"pooly_system_uptime_seconds", uptimeSeconds, "seconds"},
		{"pooly_system_boot_time_timestamp_seconds", float64(bootTime), "timestamp_seconds"},
		{"pooly_system_boot_id_changed", changed, "state"},
	} {
		if err := mustMetric(&metrics, item.name, item.value, MetricGauge, item.unit, nil, ts); err != nil {
			return failureObservation("uptime", "system", started, ErrorInternal, err.Error())
		}
	}
	summary := "uptime collected"
	if !ok {
		summary = "boot baseline recorded"
	}
	if changed == 1 {
		summary = "boot id changed"
	}
	return successObservation("uptime", "system", started, metrics, summary)
}
