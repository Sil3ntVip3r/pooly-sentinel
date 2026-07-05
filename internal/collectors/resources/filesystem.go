package resources

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"
)

type FSStat struct {
	SizeBytes       uint64
	FreeBytes       uint64
	AvailableBytes  uint64
	UsedBytes       uint64
	UsedRatio       float64
	InodesTotal     uint64
	InodesFree      uint64
	InodesUsedRatio float64
	ReadOnly        bool
	Type            string
}

type rawFSStat struct {
	Blocks      uint64
	BlockSize   uint64
	FreeBlocks  uint64
	AvailBlocks uint64
	Files       uint64
	FreeFiles   uint64
	ReadOnly    bool
	Type        string
}

func ComputeFSStat(raw rawFSStat) (FSStat, error) {
	if raw.Blocks == 0 || raw.BlockSize == 0 {
		return FSStat{}, fmt.Errorf("filesystem has zero blocks or block size")
	}
	size := raw.Blocks * raw.BlockSize
	free := raw.FreeBlocks * raw.BlockSize
	available := raw.AvailBlocks * raw.BlockSize
	used := uint64(0)
	if size >= free {
		used = size - free
	}
	inodesUsedRatio := 0.0
	if raw.Files > 0 {
		usedInodes := uint64(0)
		if raw.Files >= raw.FreeFiles {
			usedInodes = raw.Files - raw.FreeFiles
		}
		inodesUsedRatio = ratio(usedInodes, raw.Files)
	}
	return FSStat{
		SizeBytes:       size,
		FreeBytes:       free,
		AvailableBytes:  available,
		UsedBytes:       used,
		UsedRatio:       1 - ratio(available, size),
		InodesTotal:     raw.Files,
		InodesFree:      raw.FreeFiles,
		InodesUsedRatio: inodesUsedRatio,
		ReadOnly:        raw.ReadOnly,
		Type:            raw.Type,
	}, nil
}

func CollectFilesystems(ctx context.Context, opts Options) []Observation {
	mounts, err := normalizeMounts(opts.FilesystemMounts)
	if err != nil {
		return []Observation{failureObservation("filesystem", "all", time.Now(), ErrorInternal, err.Error())}
	}
	var observations []Observation
	for _, mount := range mounts {
		observations = append(observations, collectFilesystem(ctx, mount))
	}
	return observations
}

func collectFilesystem(ctx context.Context, mount string) Observation {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return failureObservation("filesystem", mount, started, ErrorTimeout, err.Error())
	}
	raw, err := statFilesystem(mount)
	if err != nil {
		class := ErrorInternal
		if err == errFilesystemUnsupported {
			class = ErrorUnsupported
		}
		return failureObservation("filesystem", mount, started, class, err.Error())
	}
	stat, err := ComputeFSStat(raw)
	if err != nil {
		return failureObservation("filesystem", mount, started, ErrorParse, err.Error())
	}
	ts := started.UTC()
	labels := map[string]string{"mount": mount}
	var metrics []Metric
	for _, item := range []struct {
		name  string
		value float64
		unit  string
	}{
		{"pooly_filesystem_size_bytes", float64(stat.SizeBytes), "bytes"},
		{"pooly_filesystem_available_bytes", float64(stat.AvailableBytes), "bytes"},
		{"pooly_filesystem_free_bytes", float64(stat.FreeBytes), "bytes"},
		{"pooly_filesystem_used_bytes", float64(stat.UsedBytes), "bytes"},
		{"pooly_filesystem_used_ratio", stat.UsedRatio, "ratio"},
		{"pooly_filesystem_inodes_total", float64(stat.InodesTotal), "count"},
		{"pooly_filesystem_inodes_free", float64(stat.InodesFree), "count"},
		{"pooly_filesystem_inodes_used_ratio", stat.InodesUsedRatio, "ratio"},
		{"pooly_filesystem_readonly", boolFloat(stat.ReadOnly), "state"},
	} {
		if err := mustMetric(&metrics, item.name, item.value, MetricGauge, item.unit, labels, ts); err != nil {
			return failureObservation("filesystem", mount, started, ErrorInternal, err.Error())
		}
	}
	return successObservation("filesystem", mount, started, metrics, "filesystem collected")
}

func normalizeMounts(mounts []string) ([]string, error) {
	return uniqueNormalized(mounts, func(value string) (string, error) {
		if value == "" {
			return "", fmt.Errorf("mount path is empty")
		}
		if !filepath.IsAbs(value) {
			return "", fmt.Errorf("mount path must be absolute")
		}
		return filepath.Clean(value), nil
	})
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func dedupeStrings(values []string) []string {
	var out []string
	for _, value := range values {
		if !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}
