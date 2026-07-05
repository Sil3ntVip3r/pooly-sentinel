package resources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type PSIRecord struct {
	Type   string
	Avg10  float64
	Avg60  float64
	Avg300 float64
	Total  uint64
}

func ParsePSI(data []byte) (map[string]PSIRecord, error) {
	records := map[string]PSIRecord{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return nil, fmt.Errorf("psi line %q is truncated", fields[0])
		}
		record := PSIRecord{Type: fields[0]}
		if record.Type != "some" && record.Type != "full" {
			return nil, fmt.Errorf("unknown psi record type %q", record.Type)
		}
		for _, field := range fields[1:] {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				return nil, fmt.Errorf("psi field %q is malformed", field)
			}
			switch key {
			case "avg10":
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return nil, fmt.Errorf("psi avg10 is malformed")
				}
				record.Avg10 = parsed
			case "avg60":
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return nil, fmt.Errorf("psi avg60 is malformed")
				}
				record.Avg60 = parsed
			case "avg300":
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return nil, fmt.Errorf("psi avg300 is malformed")
				}
				record.Avg300 = parsed
			case "total":
				parsed, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("psi total is malformed")
				}
				record.Total = parsed
			default:
				return nil, fmt.Errorf("unknown psi field %q", key)
			}
		}
		records[record.Type] = record
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no psi records found")
	}
	return records, nil
}

func CollectPressure(ctx context.Context, opts Options) []Observation {
	var observations []Observation
	for _, area := range []string{"cpu", "memory", "io"} {
		observations = append(observations, collectPressureArea(ctx, opts, area))
	}
	return observations
}

func collectPressureArea(ctx context.Context, opts Options, area string) Observation {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return failureObservation("pressure", area, started, ErrorTimeout, err.Error())
	}
	data, err := opts.Source.ReadFile("/proc/pressure/" + area)
	if err != nil {
		class := classifyReadError(err)
		if class == ErrorSourceMissing && opts.PressureMissingOK {
			return failureObservation("pressure", area, started, ErrorUnsupported, "psi file unavailable")
		}
		return failureObservation("pressure", area, started, class, "read psi failed")
	}
	records, err := ParsePSI(data)
	if err != nil {
		return failureObservation("pressure", area, started, ErrorParse, err.Error())
	}
	ts := started.UTC()
	var metrics []Metric
	for _, record := range records {
		labels := map[string]string{"pressure_type": record.Type}
		for _, item := range []struct {
			name  string
			value float64
			unit  string
			kind  MetricKind
			win   string
		}{
			{fmt.Sprintf("pooly_pressure_%s_%s_avg10", area, record.Type), record.Avg10, "percent", MetricGauge, "avg10"},
			{fmt.Sprintf("pooly_pressure_%s_%s_avg60", area, record.Type), record.Avg60, "percent", MetricGauge, "avg60"},
			{fmt.Sprintf("pooly_pressure_%s_%s_avg300", area, record.Type), record.Avg300, "percent", MetricGauge, "avg300"},
			{fmt.Sprintf("pooly_pressure_%s_%s_total_microseconds", area, record.Type), float64(record.Total), "microseconds", MetricCounter, ""},
		} {
			itemLabels := labels
			if item.win != "" {
				itemLabels = map[string]string{"pressure_type": record.Type, "window": item.win}
			}
			if err := mustMetric(&metrics, item.name, item.value, item.kind, item.unit, itemLabels, ts); err != nil {
				return failureObservation("pressure", area, started, ErrorInternal, err.Error())
			}
		}
	}
	return successObservation("pressure", area, started, metrics, "psi collected")
}
