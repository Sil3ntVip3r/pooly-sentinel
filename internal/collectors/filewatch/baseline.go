package filewatch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/platform"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

type Target struct {
	Name                string
	Path                string
	Type                string
	Hash                bool
	Manifest            bool
	AllowPrivateKeyHash bool
}

type Snapshot struct {
	Name               string `json:"name"`
	Path               string `json:"path"`
	Exists             bool   `json:"exists"`
	Type               string `json:"type"`
	UID                int64  `json:"uid"`
	GID                int64  `json:"gid"`
	Mode               string `json:"mode"`
	Size               int64  `json:"size"`
	ModTimeUnix        int64  `json:"mod_time_unix"`
	Hash               string `json:"hash,omitempty"`
	HashComplete       bool   `json:"hash_complete,omitempty"`
	Oversized          bool   `json:"oversized,omitempty"`
	SourceChanged      bool   `json:"source_changed,omitempty"`
	ManifestHash       string `json:"manifest_hash,omitempty"`
	ManifestComplete   bool   `json:"manifest_complete,omitempty"`
	ManifestTruncated  bool   `json:"manifest_truncated,omitempty"`
	EntryCountObserved int    `json:"entry_count_observed,omitempty"`
	EntryLimit         int    `json:"entry_limit,omitempty"`
	IncompleteReason   string `json:"incomplete_reason,omitempty"`
}

type OpenedFile interface {
	io.Reader
	Close() error
	Stat() (fs.FileInfo, error)
}

type FileSource interface {
	Lstat(name string) (fs.FileInfo, error)
	OpenNoFollow(name string) (OpenedFile, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

type OSFileSource struct{}

func (OSFileSource) Lstat(name string) (fs.FileInfo, error)       { return os.Lstat(name) }
func (OSFileSource) OpenNoFollow(name string) (OpenedFile, error) { return openNoFollow(name) }
func (OSFileSource) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

type Options struct {
	Targets             []Target
	Timeout             time.Duration
	MaxFileBytes        int64
	MaxDirectoryEntries int
	Source              FileSource
	State               resources.StateStore
	Persist             bool
	PlatformSupported   *bool
}

func DefaultOptions() Options {
	return Options{
		Timeout:             3 * time.Second,
		MaxFileBytes:        1024 * 1024,
		MaxDirectoryEntries: 256,
		Source:              OSFileSource{},
		PlatformSupported:   nil,
	}
}

func Collect(ctx context.Context, opts Options) []resources.Observation {
	opts = optionsWithDefaults(opts)
	started := time.Now()
	if ctx == nil {
		return []resources.Observation{failureObservation("filewatch", "all", started, resources.ErrorInternal, "context is nil")}
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return []resources.Observation{failureObservation("filewatch", "all", started, resources.ErrorTimeout, err.Error())}
	}
	if !platform.Supported(opts.PlatformSupported) {
		if len(opts.Targets) == 0 {
			return []resources.Observation{unsupportedObservation("filewatch", "all")}
		}
		observations := make([]resources.Observation, 0, len(opts.Targets))
		for _, target := range opts.Targets {
			observations = append(observations, unsupportedObservation("filewatch", targetName(target)))
		}
		return observations
	}
	if len(opts.Targets) == 0 {
		return []resources.Observation{successObservation("filewatch", "all", started, nil, "no filewatch targets configured")}
	}
	observations := make([]resources.Observation, 0, len(opts.Targets))
	for _, target := range opts.Targets {
		observations = append(observations, collectTarget(ctx, opts, target))
	}
	return observations
}

func collectTarget(ctx context.Context, opts Options, target Target) resources.Observation {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return failureObservation("filewatch", targetName(target), started, resources.ErrorTimeout, err.Error())
	}
	snapshot, err := SnapshotTarget(ctx, opts.Source, target, opts.MaxFileBytes, opts.MaxDirectoryEntries)
	if err != nil {
		return failureObservation("filewatch", targetName(target), started, classifyFileError(err), err.Error())
	}
	if snapshot.IncompleteReason != "" {
		return incompleteObservation(target, snapshot, started)
	}
	if err := ctx.Err(); err != nil {
		return failureObservation("filewatch", targetName(target), started, resources.ErrorTimeout, err.Error())
	}
	previous, ok, stateErr := loadSnapshot(ctx, opts.State, targetName(target))
	if stateErr != nil {
		if ok {
			if err := ctx.Err(); err != nil {
				return failureObservation("filewatch", targetName(target), started, resources.ErrorTimeout, err.Error())
			}
			if saveErr := saveSnapshot(ctx, opts.State, opts.Persist, targetName(target), snapshot); saveErr != nil {
				return failureObservation("filewatch", targetName(target), started, resources.ErrorState, "save file baseline failed")
			}
			obs := snapshotObservation(target, snapshot, started, "baseline reset")
			obs.Stale = true
			obs.ErrorClass = resources.ErrorCounterReset
			return obs
		}
		return failureObservation("filewatch", targetName(target), started, resources.ErrorState, "load file baseline failed")
	}
	change := "baseline_recorded"
	if ok {
		change = compareSnapshots(previous, snapshot)
	}
	if err := ctx.Err(); err != nil {
		return failureObservation("filewatch", targetName(target), started, resources.ErrorTimeout, err.Error())
	}
	if err := saveSnapshot(ctx, opts.State, opts.Persist, targetName(target), snapshot); err != nil {
		return failureObservation("filewatch", targetName(target), started, resources.ErrorState, "save file baseline failed")
	}
	return snapshotObservation(target, snapshot, started, change)
}

func SnapshotTarget(ctx context.Context, source FileSource, target Target, maxFileBytes int64, maxDirectoryEntries int) (Snapshot, error) {
	if source == nil {
		source = OSFileSource{}
	}
	if ctx == nil {
		return Snapshot{}, context.Canceled
	}
	cleanPath := filepath.Clean(target.Path)
	snapshot := Snapshot{Name: targetName(target), Path: cleanPath}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	info, err := source.Lstat(cleanPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return snapshot, nil
		}
		return Snapshot{}, err
	}
	snapshot.Exists = true
	snapshot.Type = fileType(info)
	snapshot.Mode = info.Mode().Perm().String()
	snapshot.Size = info.Size()
	snapshot.ModTimeUnix = info.ModTime().UTC().Unix()
	snapshot.UID, snapshot.GID = owner(info)
	if snapshot.Type == "symlink" {
		snapshot.IncompleteReason = "symlink_rejected"
		return snapshot, nil
	}
	if reason := targetTypeMismatch(target.Type, snapshot.Type); reason != "" {
		snapshot.IncompleteReason = reason
		return snapshot, nil
	}
	if snapshot.Type == "file" && target.Hash {
		if maxFileBytes <= 0 {
			maxFileBytes = 1024 * 1024
		}
		if info.Size() > maxFileBytes {
			snapshot.Oversized = true
			snapshot.IncompleteReason = "file_oversized"
			return snapshot, nil
		}
		if looksPrivateKeyPath(cleanPath) && !target.AllowPrivateKeyHash {
			return snapshot, nil
		}
		data, changed, err := readBoundedRegular(ctx, source, cleanPath, info, maxFileBytes)
		if err != nil {
			return Snapshot{}, err
		}
		if changed {
			snapshot.SourceChanged = true
			snapshot.IncompleteReason = "source_changed"
			return snapshot, nil
		}
		if int64(len(data)) > maxFileBytes {
			snapshot.Oversized = true
			snapshot.IncompleteReason = "file_oversized"
			return snapshot, nil
		}
		snapshot.Hash = hashBytes(data)
		snapshot.HashComplete = true
	}
	if snapshot.Type == "directory" && target.Manifest {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		entries, err := source.ReadDir(cleanPath)
		if err != nil {
			return Snapshot{}, err
		}
		hash, observed, truncated, err := manifestHash(entries, maxDirectoryEntries)
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.ManifestHash = hash
		snapshot.ManifestComplete = !truncated
		snapshot.ManifestTruncated = truncated
		snapshot.EntryCountObserved = observed
		snapshot.EntryLimit = maxDirectoryEntries
		if truncated {
			snapshot.IncompleteReason = "manifest_truncated"
		}
	}
	return snapshot, nil
}

func readBoundedRegular(ctx context.Context, source FileSource, path string, initial fs.FileInfo, maxFileBytes int64) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	opened, err := source.OpenNoFollow(path)
	if err != nil {
		return nil, false, err
	}
	if opened == nil {
		return nil, false, errors.New("open returned nil file")
	}
	defer opened.Close()
	openedInfo, err := opened.Stat()
	if err != nil {
		return nil, false, err
	}
	if fileType(openedInfo) != "file" {
		return nil, true, nil
	}
	if !sameObject(initial, openedInfo) {
		return nil, true, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(io.LimitReader(opened, maxFileBytes+1))
	if err != nil {
		return nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return data, false, nil
}

func loadSnapshot(ctx context.Context, store resources.StateStore, target string) (Snapshot, bool, error) {
	if store == nil {
		return Snapshot{}, false, nil
	}
	raw, err := store.Get(ctx, "filewatch", target)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return Snapshot{}, false, nil
		}
		return Snapshot{}, false, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return Snapshot{}, true, err
	}
	return snapshot, true, nil
}

func saveSnapshot(ctx context.Context, store resources.StateStore, persist bool, target string, snapshot Snapshot) error {
	if !persist || store == nil {
		return nil
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return store.Upsert(ctx, "filewatch", target, "ok", string(data))
}

func snapshotObservation(target Target, snapshot Snapshot, started time.Time, change string) resources.Observation {
	metrics, err := snapshotMetrics(target, snapshot, started.UTC(), change)
	if err != nil {
		return failureObservation("filewatch", targetName(target), started, resources.ErrorInternal, err.Error())
	}
	obs := successObservation("filewatch", targetName(target), started, metrics, "file state "+change)
	obs.Fields = map[string]string{
		"name":                 targetName(target),
		"path":                 redaction.Redact(snapshot.Path),
		"type":                 snapshot.Type,
		"exists":               strconv.FormatBool(snapshot.Exists),
		"uid":                  strconv.FormatInt(snapshot.UID, 10),
		"gid":                  strconv.FormatInt(snapshot.GID, 10),
		"mode":                 snapshot.Mode,
		"size":                 strconv.FormatInt(snapshot.Size, 10),
		"mod_time_unix":        strconv.FormatInt(snapshot.ModTimeUnix, 10),
		"change":               change,
		"hash_complete":        strconv.FormatBool(snapshot.HashComplete),
		"oversized":            strconv.FormatBool(snapshot.Oversized),
		"source_changed":       strconv.FormatBool(snapshot.SourceChanged),
		"manifest_complete":    strconv.FormatBool(snapshot.ManifestComplete),
		"manifest_truncated":   strconv.FormatBool(snapshot.ManifestTruncated),
		"entry_count_observed": strconv.Itoa(snapshot.EntryCountObserved),
		"entry_limit":          strconv.Itoa(snapshot.EntryLimit),
	}
	if snapshot.IncompleteReason != "" {
		obs.Fields["incomplete_reason"] = snapshot.IncompleteReason
	}
	if snapshot.Hash != "" {
		obs.Fields["hash_sha256"] = snapshot.Hash
	}
	if snapshot.ManifestHash != "" {
		obs.Fields["manifest_sha256"] = snapshot.ManifestHash
	}
	return obs
}

func incompleteObservation(target Target, snapshot Snapshot, started time.Time) resources.Observation {
	obs := snapshotObservation(target, snapshot, started, snapshot.IncompleteReason)
	obs.Success = false
	obs.Stale = true
	obs.ErrorClass = incompleteErrorClass(snapshot.IncompleteReason)
	obs.Summary = "file state " + snapshot.IncompleteReason
	return obs
}

func snapshotMetrics(target Target, snapshot Snapshot, ts time.Time, change string) ([]resources.Metric, error) {
	labels := map[string]string{"watch": targetName(target)}
	items := []struct {
		name  string
		value float64
		unit  string
	}{
		{"pooly_filewatch_target_exists", boolFloat(snapshot.Exists), "state"},
		{"pooly_filewatch_target_changed", boolFloat(change != "unchanged" && change != "baseline_recorded"), "state"},
		{"pooly_filewatch_target_size_bytes", float64(snapshot.Size), "bytes"},
		{"pooly_filewatch_target_symlink", boolFloat(snapshot.Type == "symlink"), "state"},
		{"pooly_filewatch_manifest_complete", boolFloat(snapshot.ManifestComplete), "state"},
		{"pooly_filewatch_manifest_truncated", boolFloat(snapshot.ManifestTruncated), "state"},
		{"pooly_filewatch_target_oversized", boolFloat(snapshot.Oversized), "state"},
	}
	metrics := make([]resources.Metric, 0, len(items))
	for _, item := range items {
		metric, err := resources.NewMetric(item.name, item.value, resources.MetricGauge, item.unit, labels, ts)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func compareSnapshots(previous Snapshot, current Snapshot) string {
	switch {
	case !previous.Exists && current.Exists:
		return "created"
	case previous.Exists && !current.Exists:
		return "deleted"
	case previous.Type != current.Type:
		return "type_changed"
	case previous.Mode != current.Mode:
		return "permission_changed"
	case previous.UID != current.UID || previous.GID != current.GID:
		return "owner_changed"
	case previous.Hash != current.Hash || previous.ManifestHash != current.ManifestHash || previous.Size != current.Size || previous.ModTimeUnix != current.ModTimeUnix:
		return "modified"
	default:
		return "unchanged"
	}
}

func targetTypeMismatch(configured string, actual string) string {
	switch configured {
	case "", "any":
		return ""
	case "file":
		if actual != "file" {
			return "type_mismatch"
		}
	case "directory":
		if actual != "directory" {
			return "type_mismatch"
		}
	}
	return ""
}

func fileType(info fs.FileInfo) string {
	mode := info.Mode()
	switch {
	case mode.Type()&fs.ModeSymlink != 0:
		return "symlink"
	case mode.IsRegular():
		return "file"
	case mode.IsDir():
		return "directory"
	default:
		return "other"
	}
}

func owner(info fs.FileInfo) (int64, int64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return -1, -1
	}
	return int64(stat.Uid), int64(stat.Gid)
}

func classifyFileError(err error) resources.ErrorClass {
	if err == nil {
		return resources.ErrorNone
	}
	if errors.Is(err, fs.ErrNotExist) {
		return resources.ErrorSourceMissing
	}
	if errors.Is(err, fs.ErrPermission) || strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		return resources.ErrorPermissionDenied
	}
	if errors.Is(err, syscall.ELOOP) {
		return resources.ErrorSourceType
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return resources.ErrorTimeout
	}
	return resources.ErrorInternal
}

func incompleteErrorClass(reason string) resources.ErrorClass {
	switch reason {
	case "file_oversized":
		return resources.ErrorOversized
	case "source_changed":
		return resources.ErrorSourceChanged
	case "symlink_rejected", "type_mismatch":
		return resources.ErrorSourceType
	case "manifest_truncated":
		return resources.ErrorParse
	default:
		return resources.ErrorInternal
	}
}

func optionsWithDefaults(opts Options) Options {
	defaults := DefaultOptions()
	if opts.Timeout == 0 {
		opts.Timeout = defaults.Timeout
	}
	if opts.MaxFileBytes == 0 {
		opts.MaxFileBytes = defaults.MaxFileBytes
	}
	if opts.MaxDirectoryEntries == 0 {
		opts.MaxDirectoryEntries = defaults.MaxDirectoryEntries
	}
	if opts.Source == nil {
		opts.Source = defaults.Source
	}
	return opts
}

func targetName(target Target) string {
	if target.Name != "" {
		return safeName(target.Name)
	}
	return safeName(filepath.Base(filepath.Clean(target.Path)))
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "target"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
		if b.Len() >= 96 {
			break
		}
	}
	if b.Len() == 0 {
		return "target"
	}
	return b.String()
}

func looksPrivateKeyPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasPrefix(base, "id_") || strings.Contains(base, "private_key") || strings.Contains(base, ".pem")
}

func sameObject(a fs.FileInfo, b fs.FileInfo) bool {
	aStat, aOK := a.Sys().(*syscall.Stat_t)
	bStat, bOK := b.Sys().(*syscall.Stat_t)
	if aOK && bOK && aStat != nil && bStat != nil {
		return aStat.Dev == bStat.Dev && aStat.Ino == bStat.Ino
	}
	return a.Mode().Type() == b.Mode().Type() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func successObservation(name string, target string, started time.Time, metrics []resources.Metric, summary string) resources.Observation {
	return resources.Observation{
		Collector: name,
		Target:    target,
		Timestamp: started.UTC(),
		Duration:  time.Since(started),
		Success:   true,
		Supported: true,
		Metrics:   metrics,
		Summary:   redaction.Redact(summary),
	}
}

func failureObservation(name string, target string, started time.Time, class resources.ErrorClass, summary string) resources.Observation {
	return resources.Observation{
		Collector:  name,
		Target:     target,
		Timestamp:  started.UTC(),
		Duration:   time.Since(started),
		Success:    false,
		Supported:  class != resources.ErrorUnsupported,
		Summary:    redaction.Redact(summary),
		ErrorClass: class,
	}
}

func unsupportedObservation(name string, target string) resources.Observation {
	started := time.Now()
	return failureObservation(name, target, started, resources.ErrorUnsupported, "unsupported platform")
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
