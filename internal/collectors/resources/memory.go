package resources

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type MemInfo struct {
	Fields map[string]uint64
}

var requiredMemInfoFields = []string{
	"MemTotal", "MemAvailable", "MemFree", "Buffers", "Cached",
	"SwapTotal", "SwapFree", "Dirty", "Writeback", "Slab",
	"SReclaimable", "SUnreclaim", "KernelStack", "PageTables",
}

func ParseMemInfo(data []byte) (MemInfo, error) {
	fields := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return MemInfo{}, fmt.Errorf("meminfo line is truncated")
		}
		key := strings.TrimSuffix(parts[0], ":")
		value, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return MemInfo{}, fmt.Errorf("meminfo field %s is malformed", key)
		}
		if value > math.MaxUint64/1024 {
			return MemInfo{}, fmt.Errorf("meminfo field %s overflows bytes", key)
		}
		fields[key] = value * 1024
	}
	for _, key := range requiredMemInfoFields {
		if _, ok := fields[key]; !ok {
			return MemInfo{}, fmt.Errorf("meminfo missing required field %s", key)
		}
	}
	if fields["MemTotal"] == 0 {
		return MemInfo{}, fmt.Errorf("MemTotal must be greater than zero")
	}
	return MemInfo{Fields: fields}, nil
}

func (m MemInfo) Value(key string) uint64 {
	return m.Fields[key]
}

func CollectMemory(ctx context.Context, opts Options) Observation {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return failureObservation("memory", "all", started, ErrorTimeout, err.Error())
	}
	data, err := opts.Source.ReadFile("/proc/meminfo")
	if err != nil {
		return failureObservation("memory", "all", started, classifyReadError(err), "read /proc/meminfo failed")
	}
	mem, err := ParseMemInfo(data)
	if err != nil {
		return failureObservation("memory", "all", started, ErrorParse, err.Error())
	}
	total := mem.Value("MemTotal")
	available := mem.Value("MemAvailable")
	swapTotal := mem.Value("SwapTotal")
	swapFree := mem.Value("SwapFree")
	availableRatio := ratio(available, total)
	usedRatio := 1 - availableRatio
	swapUsedRatio := 0.0
	if swapTotal > 0 {
		swapUsedRatio = 1 - ratio(swapFree, swapTotal)
	}
	ts := started.UTC()
	var metrics []Metric
	for _, item := range []struct {
		name  string
		value float64
		unit  string
	}{
		{"pooly_memory_total_bytes", float64(total), "bytes"},
		{"pooly_memory_available_bytes", float64(available), "bytes"},
		{"pooly_memory_free_bytes", float64(mem.Value("MemFree")), "bytes"},
		{"pooly_memory_used_ratio", usedRatio, "ratio"},
		{"pooly_memory_available_ratio", availableRatio, "ratio"},
		{"pooly_swap_total_bytes", float64(swapTotal), "bytes"},
		{"pooly_swap_free_bytes", float64(swapFree), "bytes"},
		{"pooly_swap_used_ratio", swapUsedRatio, "ratio"},
		{"pooly_memory_dirty_bytes", float64(mem.Value("Dirty")), "bytes"},
		{"pooly_memory_writeback_bytes", float64(mem.Value("Writeback")), "bytes"},
		{"pooly_memory_slab_bytes", float64(mem.Value("Slab")), "bytes"},
		{"pooly_memory_buffers_bytes", float64(mem.Value("Buffers")), "bytes"},
		{"pooly_memory_cached_bytes", float64(mem.Value("Cached")), "bytes"},
		{"pooly_memory_sreclaimable_bytes", float64(mem.Value("SReclaimable")), "bytes"},
		{"pooly_memory_sunreclaim_bytes", float64(mem.Value("SUnreclaim")), "bytes"},
		{"pooly_memory_kernel_stack_bytes", float64(mem.Value("KernelStack")), "bytes"},
		{"pooly_memory_page_tables_bytes", float64(mem.Value("PageTables")), "bytes"},
	} {
		if err := mustMetric(&metrics, item.name, item.value, MetricGauge, item.unit, nil, ts); err != nil {
			return failureObservation("memory", "all", started, ErrorInternal, err.Error())
		}
	}
	return successObservation("memory", "all", started, metrics, "memory collected")
}
