package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type CPUStat struct {
	Name      string `json:"name"`
	User      uint64 `json:"user"`
	Nice      uint64 `json:"nice"`
	System    uint64 `json:"system"`
	Idle      uint64 `json:"idle"`
	IOWait    uint64 `json:"iowait"`
	IRQ       uint64 `json:"irq"`
	SoftIRQ   uint64 `json:"softirq"`
	Steal     uint64 `json:"steal"`
	Guest     uint64 `json:"guest"`
	GuestNice uint64 `json:"guest_nice"`
}

func (s CPUStat) Total() uint64 {
	return s.User + s.Nice + s.System + s.Idle + s.IOWait + s.IRQ + s.SoftIRQ + s.Steal
}

func (s CPUStat) Busy() uint64 {
	total := s.Total()
	idle := s.Idle + s.IOWait
	if idle > total {
		return 0
	}
	return total - idle
}

type cpuState struct {
	Stats map[string]CPUStat `json:"stats"`
}

func ParseProcStatCPU(data []byte) ([]CPUStat, error) {
	lines := strings.Split(string(data), "\n")
	var stats []CPUStat
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		if fields[0] != "cpu" && !isCPUName(fields[0]) {
			continue
		}
		if len(fields) < 8 {
			return nil, fmt.Errorf("cpu line %q is truncated", fields[0])
		}
		values := make([]uint64, 10)
		for i := 1; i < len(fields) && i <= 10; i++ {
			value, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("cpu line %q has malformed value", fields[0])
			}
			values[i-1] = value
		}
		stats = append(stats, CPUStat{
			Name:      fields[0],
			User:      values[0],
			Nice:      values[1],
			System:    values[2],
			Idle:      values[3],
			IOWait:    values[4],
			IRQ:       values[5],
			SoftIRQ:   values[6],
			Steal:     values[7],
			Guest:     values[8],
			GuestNice: values[9],
		})
	}
	if len(stats) == 0 {
		return nil, fmt.Errorf("no cpu lines found")
	}
	return stats, nil
}

func CPUCount(stats []CPUStat) int {
	count := 0
	for _, stat := range stats {
		if isCPUName(stat.Name) {
			count++
		}
	}
	return count
}

func isCPUName(name string) bool {
	if len(name) <= 3 || !strings.HasPrefix(name, "cpu") {
		return false
	}
	for _, r := range name[3:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func CollectCPU(ctx context.Context, opts Options) Observation {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return failureObservation("cpu", "all", started, ErrorTimeout, err.Error())
	}
	data, err := opts.Source.ReadFile("/proc/stat")
	if err != nil {
		return failureObservation("cpu", "all", started, classifyReadError(err), "read /proc/stat failed")
	}
	stats, err := ParseProcStatCPU(data)
	if err != nil {
		return failureObservation("cpu", "all", started, ErrorParse, err.Error())
	}
	current := cpuState{Stats: map[string]CPUStat{}}
	for _, stat := range stats {
		current.Stats[stat.Name] = stat
	}
	previous, ok, err := loadState[cpuState](ctx, opts.State, "cpu", "all")
	if err != nil {
		return failureObservation("cpu", "all", started, ErrorState, "load cpu baseline failed")
	}

	ts := started.UTC()
	var metrics []Metric
	if err := mustMetric(&metrics, "pooly_cpu_count", float64(CPUCount(stats)), MetricGauge, "count", nil, ts); err != nil {
		return failureObservation("cpu", "all", started, ErrorInternal, err.Error())
	}
	summary := "baseline recorded"
	errorClass := ErrorNone
	if ok {
		for _, stat := range stats {
			prev, exists := previous.Stats[stat.Name]
			if !exists {
				continue
			}
			totalDelta := CalculateCounterDelta(prev.Total(), stat.Total(), true)
			busyDelta := CalculateCounterDelta(prev.Busy(), stat.Busy(), true)
			iowaitDelta := CalculateCounterDelta(prev.IOWait, stat.IOWait, true)
			stealDelta := CalculateCounterDelta(prev.Steal, stat.Steal, true)
			if totalDelta.Reset || busyDelta.Reset || iowaitDelta.Reset || stealDelta.Reset {
				errorClass = ErrorCounterReset
				continue
			}
			if !totalDelta.Valid || totalDelta.Delta == 0 {
				continue
			}
			labels := map[string]string{"cpu": stat.Name}
			if stat.Name == "cpu" {
				labels = map[string]string{"cpu": "all"}
			}
			used := ratio(busyDelta.Delta, totalDelta.Delta)
			iowait := ratio(iowaitDelta.Delta, totalDelta.Delta)
			steal := ratio(stealDelta.Delta, totalDelta.Delta)
			for _, item := range []struct {
				name  string
				value float64
			}{
				{name: "pooly_cpu_used_ratio", value: used},
				{name: "pooly_cpu_iowait_ratio", value: iowait},
				{name: "pooly_cpu_steal_ratio", value: steal},
			} {
				if err := mustMetric(&metrics, item.name, item.value, MetricGauge, "ratio", labels, ts); err != nil {
					return failureObservation("cpu", "all", started, ErrorInternal, err.Error())
				}
			}
		}
		summary = "cpu deltas collected"
	}
	if err := saveState(ctx, opts.State, opts.Persist, "cpu", "all", current); err != nil {
		return failureObservation("cpu", "all", started, ErrorState, "save cpu baseline failed")
	}
	obs := successObservation("cpu", "all", started, metrics, summary)
	if errorClass == ErrorCounterReset {
		obs.Stale = true
		obs.ErrorClass = ErrorCounterReset
		obs.Summary = "cpu counter reset detected; baseline refreshed"
	}
	return obs
}

func ratio(numerator uint64, denominator uint64) float64 {
	if denominator == 0 {
		return 0
	}
	value := float64(numerator) / float64(denominator)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func cpuStateFromStats(stats []CPUStat) string {
	state := cpuState{Stats: map[string]CPUStat{}}
	for _, stat := range stats {
		state.Stats[stat.Name] = stat
	}
	data, _ := json.Marshal(state)
	return string(data)
}
