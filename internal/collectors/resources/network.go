package resources

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"
)

type NetworkStat struct {
	Interface string  `json:"interface"`
	RXBytes   uint64  `json:"rx_bytes"`
	TXBytes   uint64  `json:"tx_bytes"`
	RXPackets uint64  `json:"rx_packets"`
	TXPackets uint64  `json:"tx_packets"`
	RXErrors  uint64  `json:"rx_errors"`
	TXErrors  uint64  `json:"tx_errors"`
	RXDropped uint64  `json:"rx_dropped"`
	TXDropped uint64  `json:"tx_dropped"`
	OperState string  `json:"operstate"`
	Carrier   *uint64 `json:"carrier,omitempty"`
	MTU       uint64  `json:"mtu"`
}

type networkState struct {
	Stats map[string]NetworkStat `json:"stats"`
	Daily map[string]struct {
		RXBytes DailyCounter `json:"rx_bytes"`
		TXBytes DailyCounter `json:"tx_bytes"`
	} `json:"daily"`
}

func CollectNetwork(ctx context.Context, opts Options) []Observation {
	started := time.Now()
	stats, err := discoverNetworkStats(ctx, opts)
	if err != nil {
		return []Observation{failureObservation("network", "all", started, classifyReadError(err), err.Error())}
	}
	prev, hasPrev, stateErr := loadState[networkState](ctx, opts.State, "network", "all")
	if stateErr != nil {
		return []Observation{failureObservation("network", "all", started, ErrorState, "load network baseline failed")}
	}
	current := networkState{Stats: map[string]NetworkStat{}, Daily: map[string]struct {
		RXBytes DailyCounter `json:"rx_bytes"`
		TXBytes DailyCounter `json:"tx_bytes"`
	}{}}
	if hasPrev && prev.Daily != nil {
		current.Daily = prev.Daily
	}
	var observations []Observation
	for _, stat := range stats {
		current.Stats[stat.Interface] = stat
		obs := networkObservation(started, stat)
		if hasPrev {
			prevStat, ok := prev.Stats[stat.Interface]
			rxDelta := CalculateCounterDelta(prevStat.RXBytes, stat.RXBytes, ok)
			txDelta := CalculateCounterDelta(prevStat.TXBytes, stat.TXBytes, ok)
			daily := current.Daily[stat.Interface]
			daily.RXBytes = UpdateDailyCounter(daily.RXBytes, started, rxDelta)
			daily.TXBytes = UpdateDailyCounter(daily.TXBytes, started, txDelta)
			current.Daily[stat.Interface] = daily
			if rxDelta.Reset || txDelta.Reset {
				obs.Stale = true
				obs.ErrorClass = ErrorCounterReset
				obs.Summary = "network counter reset detected; baseline refreshed"
			}
			_ = mustMetric(&obs.Metrics, "pooly_network_daily_receive_bytes", float64(daily.RXBytes.Total), MetricGauge, "bytes", map[string]string{"interface": stat.Interface}, started.UTC())
			_ = mustMetric(&obs.Metrics, "pooly_network_daily_transmit_bytes", float64(daily.TXBytes.Total), MetricGauge, "bytes", map[string]string{"interface": stat.Interface}, started.UTC())
		}
		observations = append(observations, obs)
	}
	if err := saveState(ctx, opts.State, opts.Persist, "network", "all", current); err != nil {
		return []Observation{failureObservation("network", "all", started, ErrorState, "save network baseline failed")}
	}
	return observations
}

func discoverNetworkStats(ctx context.Context, opts Options) ([]NetworkStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := opts.Source.ReadDir("/sys/class/net")
	if err != nil {
		return nil, err
	}
	var stats []NetworkStat
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		iface := entry.Name()
		if !includeName(iface, opts.NetworkInclude, opts.NetworkExclude) {
			continue
		}
		stat, err := readNetworkStat(opts.Source, iface)
		if err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

func readNetworkStat(source FileSource, iface string) (NetworkStat, error) {
	base := "/sys/class/net/" + iface
	read := func(name string) (uint64, error) {
		return readUintFile(source, base+"/"+name)
	}
	stat := NetworkStat{Interface: iface}
	var err error
	if stat.RXBytes, err = read("statistics/rx_bytes"); err != nil {
		return NetworkStat{}, err
	}
	if stat.TXBytes, err = read("statistics/tx_bytes"); err != nil {
		return NetworkStat{}, err
	}
	if stat.RXPackets, err = read("statistics/rx_packets"); err != nil {
		return NetworkStat{}, err
	}
	if stat.TXPackets, err = read("statistics/tx_packets"); err != nil {
		return NetworkStat{}, err
	}
	if stat.RXErrors, err = read("statistics/rx_errors"); err != nil {
		return NetworkStat{}, err
	}
	if stat.TXErrors, err = read("statistics/tx_errors"); err != nil {
		return NetworkStat{}, err
	}
	if stat.RXDropped, err = read("statistics/rx_dropped"); err != nil {
		return NetworkStat{}, err
	}
	if stat.TXDropped, err = read("statistics/tx_dropped"); err != nil {
		return NetworkStat{}, err
	}
	if stat.MTU, err = read("mtu"); err != nil {
		return NetworkStat{}, err
	}
	operstate, err := source.ReadFile(base + "/operstate")
	if err != nil {
		return NetworkStat{}, err
	}
	stat.OperState = strings.TrimSpace(string(operstate))
	carrier, err := read("carrier")
	if err == nil {
		stat.Carrier = &carrier
	} else if !isMissing(err) {
		return NetworkStat{}, err
	}
	return stat, nil
}

func networkObservation(started time.Time, stat NetworkStat) Observation {
	ts := started.UTC()
	labels := map[string]string{"interface": stat.Interface}
	carrier := -1.0
	if stat.Carrier != nil {
		carrier = float64(*stat.Carrier)
	}
	up := -1.0
	switch stat.OperState {
	case "up":
		up = 1
	case "down", "lowerlayerdown", "dormant", "notpresent", "testing":
		up = 0
	}
	var metrics []Metric
	for _, item := range []struct {
		name  string
		value float64
		unit  string
		kind  MetricKind
	}{
		{"pooly_network_receive_bytes_total", float64(stat.RXBytes), "bytes", MetricCounter},
		{"pooly_network_transmit_bytes_total", float64(stat.TXBytes), "bytes", MetricCounter},
		{"pooly_network_receive_packets_total", float64(stat.RXPackets), "count", MetricCounter},
		{"pooly_network_transmit_packets_total", float64(stat.TXPackets), "count", MetricCounter},
		{"pooly_network_receive_errors_total", float64(stat.RXErrors), "count", MetricCounter},
		{"pooly_network_transmit_errors_total", float64(stat.TXErrors), "count", MetricCounter},
		{"pooly_network_receive_dropped_total", float64(stat.RXDropped), "count", MetricCounter},
		{"pooly_network_transmit_dropped_total", float64(stat.TXDropped), "count", MetricCounter},
		{"pooly_network_interface_up", up, "state", MetricGauge},
		{"pooly_network_interface_carrier", carrier, "state", MetricGauge},
		{"pooly_network_interface_mtu_bytes", float64(stat.MTU), "bytes", MetricGauge},
	} {
		_ = mustMetric(&metrics, item.name, item.value, item.kind, item.unit, labels, ts)
	}
	return successObservation("network", stat.Interface, started, metrics, fmt.Sprintf("network interface %s collected", stat.OperState))
}

func isMissing(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
