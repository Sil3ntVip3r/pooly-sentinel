package resources

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"
)

type DiskStat struct {
	Device               string `json:"device"`
	Reads                uint64 `json:"reads"`
	SectorsRead          uint64 `json:"sectors_read"`
	ReadMilliseconds     uint64 `json:"read_milliseconds"`
	Writes               uint64 `json:"writes"`
	SectorsWritten       uint64 `json:"sectors_written"`
	WriteMilliseconds    uint64 `json:"write_milliseconds"`
	IOInProgress         uint64 `json:"io_in_progress"`
	IOMilliseconds       uint64 `json:"io_milliseconds"`
	WeightedMilliseconds uint64 `json:"weighted_milliseconds"`
}

type diskState struct {
	Stats map[string]DiskStat `json:"stats"`
	Daily map[string]struct {
		ReadBytes  DailyCounter `json:"read_bytes"`
		WriteBytes DailyCounter `json:"write_bytes"`
	} `json:"daily"`
}

func ParseDiskStats(data []byte) ([]DiskStat, error) {
	var stats []DiskStat
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) < 14 {
			return nil, fmt.Errorf("diskstats line is truncated")
		}
		device := fields[2]
		values, err := parseDiskFields(fields[3:])
		if err != nil {
			return nil, err
		}
		values.Device = device
		stats = append(stats, values)
	}
	if len(stats) == 0 {
		return nil, fmt.Errorf("no diskstats records found")
	}
	return stats, nil
}

func ParseSysBlockStat(device string, data []byte) (DiskStat, error) {
	fields := strings.Fields(string(data))
	if len(fields) < 11 {
		return DiskStat{}, fmt.Errorf("block stat for %s is truncated", device)
	}
	stat, err := parseDiskFields(fields)
	if err != nil {
		return DiskStat{}, err
	}
	stat.Device = device
	return stat, nil
}

func parseDiskFields(fields []string) (DiskStat, error) {
	if len(fields) < 11 {
		return DiskStat{}, fmt.Errorf("disk fields are truncated")
	}
	values := make([]uint64, len(fields))
	for i, field := range fields {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return DiskStat{}, fmt.Errorf("disk field %d is malformed", i)
		}
		values[i] = value
	}
	return DiskStat{
		Reads:                values[0],
		SectorsRead:          values[2],
		ReadMilliseconds:     values[3],
		Writes:               values[4],
		SectorsWritten:       values[6],
		WriteMilliseconds:    values[7],
		IOInProgress:         values[8],
		IOMilliseconds:       values[9],
		WeightedMilliseconds: values[10],
	}, nil
}

func (d DiskStat) ReadBytes() uint64  { return d.SectorsRead * 512 }
func (d DiskStat) WriteBytes() uint64 { return d.SectorsWritten * 512 }

func CollectDiskIO(ctx context.Context, opts Options) []Observation {
	started := time.Now()
	stats, err := discoverDiskStats(ctx, opts)
	if err != nil {
		return []Observation{failureObservation("diskio", "all", started, classifyReadError(err), err.Error())}
	}
	stats = FilterDiskDevices(stats, opts.DiskExclude)
	prev, hasPrev, stateErr := loadState[diskState](ctx, opts.State, "diskio", "all")
	if stateErr != nil {
		return []Observation{failureObservation("diskio", "all", started, ErrorState, "load disk baseline failed")}
	}
	current := diskState{Stats: map[string]DiskStat{}, Daily: map[string]struct {
		ReadBytes  DailyCounter `json:"read_bytes"`
		WriteBytes DailyCounter `json:"write_bytes"`
	}{}}
	if hasPrev && prev.Daily != nil {
		current.Daily = prev.Daily
	}
	var observations []Observation
	for _, stat := range stats {
		current.Stats[stat.Device] = stat
		obs := diskObservation(started, stat, prev, hasPrev)
		if hasPrev {
			prevStat, ok := prev.Stats[stat.Device]
			readDelta := CalculateCounterDelta(prevStat.ReadBytes(), stat.ReadBytes(), ok)
			writeDelta := CalculateCounterDelta(prevStat.WriteBytes(), stat.WriteBytes(), ok)
			daily := current.Daily[stat.Device]
			daily.ReadBytes = UpdateDailyCounter(daily.ReadBytes, started, readDelta)
			daily.WriteBytes = UpdateDailyCounter(daily.WriteBytes, started, writeDelta)
			current.Daily[stat.Device] = daily
			if readDelta.Reset || writeDelta.Reset {
				obs.Stale = true
				obs.ErrorClass = ErrorCounterReset
				obs.Summary = "disk counter reset detected; baseline refreshed"
			}
			_ = mustMetric(&obs.Metrics, "pooly_disk_daily_read_bytes", float64(daily.ReadBytes.Total), MetricGauge, "bytes", map[string]string{"device": stat.Device}, started.UTC())
			_ = mustMetric(&obs.Metrics, "pooly_disk_daily_write_bytes", float64(daily.WriteBytes.Total), MetricGauge, "bytes", map[string]string{"device": stat.Device}, started.UTC())
		}
		observations = append(observations, obs)
	}
	if err := saveState(ctx, opts.State, opts.Persist, "diskio", "all", current); err != nil {
		return []Observation{failureObservation("diskio", "all", started, ErrorState, "save disk baseline failed")}
	}
	return observations
}

func discoverDiskStats(ctx context.Context, opts Options) ([]DiskStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.DiskAutoDiscover {
		entries, err := opts.Source.ReadDir("/sys/block")
		if err == nil {
			var stats []DiskStat
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				device := entry.Name()
				data, err := opts.Source.ReadFile("/sys/block/" + device + "/stat")
				if err != nil {
					continue
				}
				stat, err := ParseSysBlockStat(device, data)
				if err != nil {
					return nil, err
				}
				stats = append(stats, stat)
			}
			if len(stats) > 0 {
				return stats, nil
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	data, err := opts.Source.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, err
	}
	return ParseDiskStats(data)
}

func diskObservation(started time.Time, stat DiskStat, previous diskState, hasPrevious bool) Observation {
	ts := started.UTC()
	labels := map[string]string{"device": stat.Device}
	var metrics []Metric
	for _, item := range []struct {
		name  string
		value float64
		unit  string
		kind  MetricKind
	}{
		{"pooly_disk_read_bytes_total", float64(stat.ReadBytes()), "bytes", MetricCounter},
		{"pooly_disk_write_bytes_total", float64(stat.WriteBytes()), "bytes", MetricCounter},
		{"pooly_disk_reads_total", float64(stat.Reads), "count", MetricCounter},
		{"pooly_disk_writes_total", float64(stat.Writes), "count", MetricCounter},
		{"pooly_disk_read_time_seconds_total", float64(stat.ReadMilliseconds) / 1000, "seconds", MetricCounter},
		{"pooly_disk_write_time_seconds_total", float64(stat.WriteMilliseconds) / 1000, "seconds", MetricCounter},
		{"pooly_disk_io_time_seconds_total", float64(stat.IOMilliseconds) / 1000, "seconds", MetricCounter},
		{"pooly_disk_weighted_io_time_seconds_total", float64(stat.WeightedMilliseconds) / 1000, "seconds", MetricCounter},
		{"pooly_disk_io_in_progress", float64(stat.IOInProgress), "count", MetricGauge},
	} {
		_ = mustMetric(&metrics, item.name, item.value, item.kind, item.unit, labels, ts)
	}
	return successObservation("diskio", stat.Device, started, metrics, "disk counters collected")
}

func FilterDiskDevices(stats []DiskStat, exclude []string) []DiskStat {
	whole := map[string]struct{}{}
	for _, stat := range stats {
		if !matchesAny(stat.Device, exclude) && partitionParent(stat.Device) == "" {
			whole[stat.Device] = struct{}{}
		}
	}
	var out []DiskStat
	for _, stat := range stats {
		if matchesAny(stat.Device, exclude) {
			continue
		}
		if parent := partitionParent(stat.Device); parent != "" {
			if _, ok := whole[parent]; ok {
				continue
			}
		}
		out = append(out, stat)
	}
	return out
}

func partitionParent(device string) string {
	if strings.HasPrefix(device, "nvme") || strings.HasPrefix(device, "mmcblk") {
		if idx := strings.LastIndex(device, "p"); idx > 0 && idx < len(device)-1 && allDigits(device[idx+1:]) {
			return device[:idx]
		}
		return ""
	}
	i := len(device) - 1
	for i >= 0 && device[i] >= '0' && device[i] <= '9' {
		i--
	}
	if i < len(device)-1 && i >= 2 {
		return device[:i+1]
	}
	return ""
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}
