package resources

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/platform"
)

func TestCPUCollectorDeltas(t *testing.T) {
	tests := []struct {
		name      string
		first     string
		second    string
		wantUsed  float64
		wantIO    float64
		wantSteal float64
		wantStale bool
	}{
		{
			name:     "normal delta",
			first:    "cpu 100 0 100 800 0 0 0 0 10 0\ncpu0 100 0 100 800 0 0 0 0 10 0\n",
			second:   "cpu 150 0 150 900 0 0 0 0 20 0\ncpu0 150 0 150 900 0 0 0 0 20 0\n",
			wantUsed: 0.5,
		},
		{
			name:     "idle system",
			first:    "cpu 0 0 0 100 0 0 0 0\ncpu0 0 0 0 100 0 0 0 0\n",
			second:   "cpu 0 0 0 200 0 0 0 0\ncpu0 0 0 0 200 0 0 0 0\n",
			wantUsed: 0,
		},
		{
			name:     "fully busy",
			first:    "cpu 100 0 0 100 0 0 0 0\ncpu0 100 0 0 100 0 0 0 0\n",
			second:   "cpu 200 0 0 100 0 0 0 0\ncpu0 200 0 0 100 0 0 0 0\n",
			wantUsed: 1,
		},
		{
			name:   "iowait",
			first:  "cpu 0 0 0 100 0 0 0 0\ncpu0 0 0 0 100 0 0 0 0\n",
			second: "cpu 0 0 0 100 100 0 0 0\ncpu0 0 0 0 100 100 0 0 0\n",
			wantIO: 1,
		},
		{
			name:      "steal",
			first:     "cpu 0 0 0 100 0 0 0 0\ncpu0 0 0 0 100 0 0 0 0\n",
			second:    "cpu 0 0 0 100 0 0 0 100\ncpu0 0 0 0 100 0 0 0 100\n",
			wantUsed:  1,
			wantSteal: 1,
		},
		{
			name:     "guest fields not double counted",
			first:    "cpu 100 10 0 100 0 0 0 0 90 9\ncpu0 100 10 0 100 0 0 0 0 90 9\n",
			second:   "cpu 200 20 0 100 0 0 0 0 180 18\ncpu0 200 20 0 100 0 0 0 0 180 18\n",
			wantUsed: 1,
		},
		{
			name:      "counter reset",
			first:     "cpu 100 0 0 100 0 0 0 0\ncpu0 100 0 0 100 0 0 0 0\n",
			second:    "cpu 50 0 0 50 0 0 0 0\ncpu0 50 0 0 50 0 0 0 0\n",
			wantStale: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewMemoryStateStore()
			opts := testOptions(map[string]string{"/proc/stat": tt.first}, state)
			first := CollectCPU(context.Background(), opts)
			if !first.Success || metricValue(first, "pooly_cpu_used_ratio", "cpu", "all") != nil {
				t.Fatalf("first sample = %+v, want baseline without usage", first)
			}
			opts.Source = MapFileSource{Files: map[string]string{"/proc/stat": tt.second}}
			second := CollectCPU(context.Background(), opts)
			if tt.wantStale {
				if !second.Stale || second.ErrorClass != ErrorCounterReset {
					t.Fatalf("second sample = %+v, want stale counter reset", second)
				}
				return
			}
			if got := metricValue(second, "pooly_cpu_used_ratio", "cpu", "all"); got == nil || math.Abs(*got-tt.wantUsed) > 0.0001 {
				t.Fatalf("used ratio = %v, want %.4f", got, tt.wantUsed)
			}
			if tt.wantIO != 0 {
				if got := metricValue(second, "pooly_cpu_iowait_ratio", "cpu", "all"); got == nil || math.Abs(*got-tt.wantIO) > 0.0001 {
					t.Fatalf("iowait ratio = %v, want %.4f", got, tt.wantIO)
				}
			}
			if tt.wantSteal != 0 {
				if got := metricValue(second, "pooly_cpu_steal_ratio", "cpu", "all"); got == nil || math.Abs(*got-tt.wantSteal) > 0.0001 {
					t.Fatalf("steal ratio = %v, want %.4f", got, tt.wantSteal)
				}
			}
		})
	}
}

func TestCPUParseErrorsAndZeroDeltaLargeCounters(t *testing.T) {
	for _, input := range []string{"cpu 1 2 3\n", "cpu one 2 3 4 5 6 7 8\n"} {
		if _, err := ParseProcStatCPU([]byte(input)); err == nil {
			t.Fatalf("ParseProcStatCPU(%q) error = nil", input)
		}
	}
	state := NewMemoryStateStore()
	large := "cpu 18446744073709551000 0 0 1 0 0 0 0\ncpu0 18446744073709551000 0 0 1 0 0 0 0\n"
	opts := testOptions(map[string]string{"/proc/stat": large}, state)
	_ = CollectCPU(context.Background(), opts)
	second := CollectCPU(context.Background(), opts)
	if second.Stale {
		t.Fatalf("zero delta should not be stale: %+v", second)
	}
}

func TestLoadAvgParsing(t *testing.T) {
	load, err := ParseLoadAvg([]byte("1.00 2.50 3.75 4/100 12345\n"))
	if err != nil {
		t.Fatalf("ParseLoadAvg() error = %v", err)
	}
	if load.Load1 != 1 || load.Load5 != 2.5 || load.Load15 != 3.75 || load.Runnable != 4 || load.TotalTasks != 100 || load.RecentPID != 12345 {
		t.Fatalf("load = %+v", load)
	}
	if _, err := ParseLoadAvg([]byte("1 2 bad 1/2 3")); err == nil {
		t.Fatal("ParseLoadAvg() malformed error = nil")
	}
}

func TestMemoryParsingAndCollector(t *testing.T) {
	mem, err := ParseMemInfo([]byte(memInfoFixture(0)))
	if err != nil {
		t.Fatalf("ParseMemInfo() error = %v", err)
	}
	if mem.Value("MemTotal") != 1024*1024 {
		t.Fatalf("MemTotal bytes = %d", mem.Value("MemTotal"))
	}
	opts := testOptions(map[string]string{"/proc/meminfo": memInfoFixture(0)}, nil)
	obs := CollectMemory(context.Background(), opts)
	if !obs.Success {
		t.Fatalf("CollectMemory() = %+v", obs)
	}
	if got := metricValue(obs, "pooly_swap_used_ratio", "", ""); got == nil || *got != 0 {
		t.Fatalf("swap used ratio = %v, want 0", got)
	}
	missing := strings.Replace(memInfoFixture(0), "MemAvailable:     512 kB\n", "", 1)
	if _, err := ParseMemInfo([]byte(missing)); err == nil {
		t.Fatal("ParseMemInfo() missing MemAvailable error = nil")
	}
}

func TestPSIParsingOptionalAndMissingFull(t *testing.T) {
	records, err := ParsePSI([]byte("some avg10=0.10 avg60=0.20 avg300=0.30 total=42\n"))
	if err != nil {
		t.Fatalf("ParsePSI() error = %v", err)
	}
	if records["some"].Total != 42 {
		t.Fatalf("psi total = %d", records["some"].Total)
	}
	if _, err := ParsePSI([]byte("some avg10=nope avg60=0 avg300=0 total=0\n")); err == nil {
		t.Fatal("ParsePSI() malformed error = nil")
	}
	opts := testOptions(map[string]string{}, nil)
	opts.PressureMissingOK = true
	obs := collectPressureArea(context.Background(), opts, "cpu")
	if obs.Supported || obs.ErrorClass != ErrorUnsupported {
		t.Fatalf("missing psi = %+v, want unsupported", obs)
	}
}

func TestFilesystemCalculations(t *testing.T) {
	stat, err := ComputeFSStat(rawFSStat{Blocks: 100, BlockSize: 10, FreeBlocks: 20, AvailBlocks: 10, Files: 10, FreeFiles: 2})
	if err != nil {
		t.Fatalf("ComputeFSStat() error = %v", err)
	}
	if stat.SizeBytes != 1000 || stat.FreeBytes != 200 || stat.AvailableBytes != 100 || math.Abs(stat.UsedRatio-0.9) > 0.001 {
		t.Fatalf("fs stat = %+v", stat)
	}
	if math.Abs(stat.InodesUsedRatio-0.8) > 0.001 {
		t.Fatalf("inode ratio = %f", stat.InodesUsedRatio)
	}
	if _, err := ComputeFSStat(rawFSStat{}); err == nil {
		t.Fatal("ComputeFSStat() zero totals error = nil")
	}
}

func TestDiskStatsParsingFilteringAndDaily(t *testing.T) {
	stats, err := ParseDiskStats([]byte("8 0 sda 10 0 20 30 40 0 50 60 0 70 80 0 0 0 0\n8 1 sda1 1 0 1 1 1 0 1 1 0 1 1\n7 0 loop0 1 0 1 1 1 0 1 1 0 1 1\n259 0 nvme0n1 1 0 2 3 4 0 5 6 0 7 8\n259 1 nvme0n1p1 1 0 2 3 4 0 5 6 0 7 8\n"))
	if err != nil {
		t.Fatalf("ParseDiskStats() error = %v", err)
	}
	filtered := FilterDiskDevices(stats, []string{"loop*"})
	var names []string
	for _, stat := range filtered {
		names = append(names, stat.Device)
	}
	if strings.Join(names, ",") != "sda,nvme0n1" {
		t.Fatalf("filtered devices = %v", names)
	}
	if filtered[0].ReadBytes() != 20*512 || filtered[0].WriteBytes() != 50*512 {
		t.Fatalf("sector conversion failed: %+v", filtered[0])
	}
	if _, err := ParseDiskStats([]byte("bad\n")); err == nil {
		t.Fatal("ParseDiskStats() truncated error = nil")
	}
}

func TestNetworkDiscoveryCarrierResetAndDaily(t *testing.T) {
	state := NewMemoryStateStore()
	files := networkFiles("eth0", 100, 200, "up", nil)
	opts := testOptions(files, state)
	opts.NetworkExclude = []string{"lo", "docker*", "veth*", "br-*"}
	opts.Source = MapFileSource{Files: files, Dirs: map[string][]fs.DirEntry{"/sys/class/net": {DirEntry{NameValue: "eth0", Dir: true}, DirEntry{NameValue: "lo", Dir: true}}}}
	first := CollectNetwork(context.Background(), opts)
	if len(first) != 1 || !first[0].Success {
		t.Fatalf("first network = %+v", first)
	}
	files2 := networkFiles("eth0", 150, 260, "up", nil)
	opts.Source = MapFileSource{Files: files2, Dirs: map[string][]fs.DirEntry{"/sys/class/net": {DirEntry{NameValue: "eth0", Dir: true}}}}
	second := CollectNetwork(context.Background(), opts)
	if got := metricValue(second[0], "pooly_network_daily_receive_bytes", "interface", "eth0"); got == nil || *got != 50 {
		t.Fatalf("daily rx = %v, want 50", got)
	}
	if got := metricValue(second[0], "pooly_network_interface_carrier", "interface", "eth0"); got == nil || *got != -1 {
		t.Fatalf("carrier = %v, want unknown -1", got)
	}
	files3 := networkFiles("eth0", 10, 20, "up", ptrUint64(1))
	opts.Source = MapFileSource{Files: files3, Dirs: map[string][]fs.DirEntry{"/sys/class/net": {DirEntry{NameValue: "eth0", Dir: true}}}}
	third := CollectNetwork(context.Background(), opts)
	if !third[0].Stale || third[0].ErrorClass != ErrorCounterReset {
		t.Fatalf("reset observation = %+v", third[0])
	}
}

func TestUptimeBootIDChange(t *testing.T) {
	state := NewMemoryStateStore()
	opts := testOptions(uptimeFiles("boot-aaaa"), state)
	first := CollectUptime(context.Background(), opts)
	if got := metricValue(first, "pooly_system_boot_id_changed", "", ""); got == nil || *got != 0 {
		t.Fatalf("first boot changed = %v", got)
	}
	opts.Source = MapFileSource{Files: uptimeFiles("boot-bbbb")}
	second := CollectUptime(context.Background(), opts)
	if got := metricValue(second, "pooly_system_boot_id_changed", "", ""); got == nil || *got != 1 {
		t.Fatalf("second boot changed = %v", got)
	}
	if _, err := ParseBootID([]byte("bad id\n")); err == nil {
		t.Fatal("ParseBootID() malformed error = nil")
	}
}

func TestDailyCounterRolloverAndReset(t *testing.T) {
	day1 := time.Date(2026, 7, 4, 23, 0, 0, 0, time.UTC)
	day2 := day1.Add(2 * time.Hour)
	counter := UpdateDailyCounter(DailyCounter{}, day1, CounterDelta{Delta: 10, Valid: true})
	counter = UpdateDailyCounter(counter, day1, CounterDelta{Delta: 5, Valid: true})
	if counter.Total != 15 {
		t.Fatalf("daily total = %d, want 15", counter.Total)
	}
	counter = UpdateDailyCounter(counter, day2, CounterDelta{Delta: 7, Valid: true})
	if counter.Total != 7 || counter.Day != "2026-07-05" {
		t.Fatalf("rolled counter = %+v", counter)
	}
	counter = UpdateDailyCounter(counter, day2, CounterDelta{Reset: true, Valid: false})
	if counter.Total != 7 {
		t.Fatalf("reset changed total = %+v", counter)
	}
}

func TestContextStateUnsupportedAndMetricValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	obs := CollectCPU(ctx, testOptions(map[string]string{"/proc/stat": "cpu 1 0 0 1 0 0 0 0\n"}, nil))
	if obs.ErrorClass != ErrorTimeout {
		t.Fatalf("canceled cpu = %+v", obs)
	}
	state := NewMemoryStateStore()
	state.SetFail(true)
	obs = CollectCPU(context.Background(), testOptions(map[string]string{"/proc/stat": "cpu 1 0 0 1 0 0 0 0\n"}, state))
	if obs.ErrorClass != ErrorState {
		t.Fatalf("state failure = %+v", obs)
	}
	all := Collect(context.Background(), Options{PlatformSupported: platform.Bool(false), CPUEnabled: true, MemoryEnabled: true})
	if len(all) == 0 || all[0].ErrorClass != ErrorUnsupported {
		t.Fatalf("unsupported collection = %+v", all)
	}
	if _, err := NewMetric("bad_metric", 1, MetricGauge, "count", nil, time.Now()); err == nil {
		t.Fatal("NewMetric() bad name error = nil")
	}
	if _, err := NewMetric("pooly_ok", 1, MetricGauge, "count", map[string]string{"path": "/tmp"}, time.Now()); err == nil {
		t.Fatal("NewMetric() disallowed label error = nil")
	}
}

func TestResourceConfigValidation(t *testing.T) {
	if _, err := pathMatch("[", "x"); err == nil {
		t.Fatal("pathMatch invalid glob error = nil")
	}
}

func testOptions(files map[string]string, state StateStore) Options {
	return Options{
		Source:              MapFileSource{Files: files},
		State:               state,
		Persist:             state != nil,
		PlatformSupported:   platform.Bool(true),
		PressureMissingOK:   true,
		CPUEnabled:          true,
		LoadEnabled:         true,
		MemoryEnabled:       true,
		PressureEnabled:     true,
		FilesystemEnabled:   true,
		DiskIOEnabled:       true,
		NetworkEnabled:      true,
		UptimeEnabled:       true,
		DiskAutoDiscover:    true,
		NetworkAutoDiscover: true,
	}
}

func metricValue(obs Observation, name string, labelKey string, labelValue string) *float64 {
	for _, metric := range obs.Metrics {
		if metric.Name != name {
			continue
		}
		if labelKey != "" && metric.Labels[labelKey] != labelValue {
			continue
		}
		value := metric.Value
		return &value
	}
	return nil
}

func memInfoFixture(swapTotal uint64) string {
	return strings.ReplaceAll(`MemTotal:        1024 kB
MemFree:          128 kB
MemAvailable:     512 kB
Buffers:           10 kB
Cached:           100 kB
SwapTotal:        SWAP kB
SwapFree:           0 kB
Dirty:              1 kB
Writeback:          2 kB
Slab:               3 kB
SReclaimable:       1 kB
SUnreclaim:         2 kB
KernelStack:        4 kB
PageTables:         5 kB
`, "SWAP", strconvFormat(swapTotal))
}

func strconvFormat(v uint64) string {
	return strconv.FormatUint(v, 10)
}

func networkFiles(iface string, rx uint64, tx uint64, state string, carrier *uint64) map[string]string {
	base := "/sys/class/net/" + iface
	files := map[string]string{
		base + "/statistics/rx_bytes":   strconvFormat(rx),
		base + "/statistics/tx_bytes":   strconvFormat(tx),
		base + "/statistics/rx_packets": "1",
		base + "/statistics/tx_packets": "2",
		base + "/statistics/rx_errors":  "0",
		base + "/statistics/tx_errors":  "0",
		base + "/statistics/rx_dropped": "0",
		base + "/statistics/tx_dropped": "0",
		base + "/operstate":             state + "\n",
		base + "/mtu":                   "1500",
	}
	if carrier != nil {
		files[base+"/carrier"] = strconvFormat(*carrier)
	}
	return files
}

func uptimeFiles(bootID string) map[string]string {
	return map[string]string{
		"/proc/uptime":                    "123.45 100.00\n",
		"/proc/stat":                      "cpu 1 0 0 1 0 0 0 0\nbtime 1710000000\n",
		"/proc/sys/kernel/random/boot_id": bootID + "\n",
	}
}

func ptrUint64(v uint64) *uint64 { return &v }

func TestCounterDelta(t *testing.T) {
	if delta := CalculateCounterDelta(10, 8, true); !delta.Reset || delta.Valid {
		t.Fatalf("reset delta = %+v", delta)
	}
	if delta := CalculateCounterDelta(8, 10, true); !delta.Valid || delta.Delta != 2 {
		t.Fatalf("normal delta = %+v", delta)
	}
	if delta := CalculateCounterDelta(0, 10, false); delta.Valid {
		t.Fatalf("first delta = %+v", delta)
	}
}

func TestMapFileSourceMissing(t *testing.T) {
	_, err := (MapFileSource{}).ReadFile("/missing")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadFile missing = %v", err)
	}
}
