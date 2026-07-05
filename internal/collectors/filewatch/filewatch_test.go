package filewatch

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/platform"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/collectors/resources"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

func TestFilewatchBaselineChangesAndDryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sshd_config")
	if err := os.WriteFile(path, []byte("PasswordAuthentication no\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := resources.NewMemoryStateStore()
	target := Target{Name: "sshd_config", Path: path, Type: "file", Hash: true}
	opts := Options{PlatformSupported: platform.Bool(true), Persist: true, State: store, Targets: []Target{target}}

	obs := Collect(context.Background(), opts)
	if len(obs) != 1 || obs[0].Fields["change"] != "baseline_recorded" {
		t.Fatalf("baseline obs = %+v", obs)
	}
	obs = Collect(context.Background(), opts)
	if len(obs) != 1 || obs[0].Fields["change"] != "unchanged" {
		t.Fatalf("unchanged obs = %+v", obs)
	}
	if err := os.WriteFile(path, []byte("PasswordAuthentication yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	obs = Collect(context.Background(), opts)
	if len(obs) != 1 || obs[0].Fields["change"] != "modified" {
		t.Fatalf("modified obs = %+v", obs)
	}
	obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Persist: false, State: store, Targets: []Target{target}})
	if len(obs) != 1 || obs[0].Fields["change"] != "unchanged" {
		t.Fatalf("dry-run changed state unexpectedly: %+v", obs)
	}
}

func TestFilewatchCreatedDeletedPermissionSymlinkAndOversize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sudoers")
	store := resources.NewMemoryStateStore()
	target := Target{Name: "sudoers", Path: path, Type: "file", Hash: true}
	opts := Options{PlatformSupported: platform.Bool(true), Persist: true, State: store, Targets: []Target{target}}

	obs := Collect(context.Background(), opts)
	if obs[0].Fields["change"] != "baseline_recorded" || obs[0].Fields["exists"] != "false" {
		t.Fatalf("missing baseline = %+v", obs)
	}
	if err := os.WriteFile(path, []byte("defaults env_reset\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	obs = Collect(context.Background(), opts)
	if obs[0].Fields["change"] != "created" {
		t.Fatalf("created obs = %+v", obs)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	obs = Collect(context.Background(), opts)
	if obs[0].Fields["change"] != "permission_changed" {
		t.Fatalf("permission obs = %+v", obs)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	obs = Collect(context.Background(), opts)
	if obs[0].Fields["change"] != "deleted" {
		t.Fatalf("deleted obs = %+v", obs)
	}

	link := filepath.Join(dir, "link")
	if err := os.Symlink("/etc/passwd", link); err == nil {
		obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Targets: []Target{{Name: "link", Path: link, Type: "any"}}})
		if obs[0].Fields["type"] != "symlink" || obs[0].ErrorClass != resources.ErrorSourceType {
			t.Fatalf("symlink obs = %+v", obs)
		}
	}

	big := filepath.Join(dir, "big")
	if err := os.WriteFile(big, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, err := SnapshotTarget(context.Background(), OSFileSource{}, Target{Name: "big", Path: big, Type: "file", Hash: true}, 4, 10)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Hash != "" {
		t.Fatalf("oversized file was hashed: %+v", snap)
	}
	if !snap.Oversized || snap.IncompleteReason != "file_oversized" {
		t.Fatalf("oversized file was not classified: %+v", snap)
	}
}

func TestFilewatchDirectoryManifestMalformedStatePermissionAndUnsupported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, err := SnapshotTarget(context.Background(), OSFileSource{}, Target{Name: "dir", Path: dir, Type: "directory", Manifest: true}, 1024, 10)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ManifestHash == "" || snap.EntryCountObserved != 1 || !snap.ManifestComplete {
		t.Fatalf("manifest snap = %+v", snap)
	}

	store := resources.NewMemoryStateStore()
	if err := store.Upsert(context.Background(), "filewatch", "dir", "ok", "{bad"); err != nil {
		t.Fatal(err)
	}
	obs := Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Persist: true, State: store, Targets: []Target{{Name: "dir", Path: dir, Type: "directory", Manifest: true}}})
	if len(obs) != 1 || !obs[0].Stale || obs[0].ErrorClass != resources.ErrorCounterReset {
		t.Fatalf("malformed state obs = %+v", obs)
	}
	raw, err := store.Get(context.Background(), "filewatch", "dir")
	if err != nil {
		t.Fatal(err)
	}
	if raw == "{bad" {
		t.Fatalf("persistent corrupt baseline was not refreshed")
	}
	if err := store.Upsert(context.Background(), "filewatch", "dir", "ok", "{bad"); err != nil {
		t.Fatal(err)
	}
	obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Persist: false, State: store, Targets: []Target{{Name: "dir", Path: dir, Type: "directory", Manifest: true}}})
	if len(obs) != 1 || !obs[0].Stale || obs[0].ErrorClass != resources.ErrorCounterReset {
		t.Fatalf("dry-run malformed state obs = %+v", obs)
	}
	raw, err = store.Get(context.Background(), "filewatch", "dir")
	if err != nil {
		t.Fatal(err)
	}
	if raw != "{bad" {
		t.Fatalf("dry-run replaced corrupt baseline: %s", raw)
	}

	obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Source: permissionSource{}, Targets: []Target{{Name: "denied", Path: "/denied", Type: "file", Hash: true}}})
	if len(obs) != 1 || obs[0].ErrorClass != resources.ErrorPermissionDenied {
		t.Fatalf("permission obs = %+v", obs)
	}

	obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(false)})
	if len(obs) != 1 || obs[0].Supported {
		t.Fatalf("unsupported obs = %+v", obs)
	}
}

type permissionSource struct{}

func (permissionSource) Lstat(name string) (fs.FileInfo, error) { return nil, fs.ErrPermission }
func (permissionSource) OpenNoFollow(name string) (OpenedFile, error) {
	return nil, fs.ErrPermission
}
func (permissionSource) ReadDir(name string) ([]fs.DirEntry, error) {
	return nil, fs.ErrPermission
}

func TestFilewatchBoundedReadAndReplacementProtection(t *testing.T) {
	base := fakeInfo{nameValue: "target", modeValue: 0o600, sizeValue: 1, modTime: time.Unix(10, 0), sysValue: &syscall.Stat_t{Dev: 1, Ino: 1}}
	cases := []struct {
		name        string
		source      fakeSource
		wantClass   resources.ErrorClass
		wantReason  string
		wantSuccess bool
	}{
		{
			name: "regular success",
			source: fakeSource{
				lstatInfo: base,
				opened:    &fakeOpenedFile{Reader: bytes.NewReader([]byte("ok")), info: base},
			},
			wantSuccess: true,
		},
		{
			name: "small reported size oversized content",
			source: fakeSource{
				lstatInfo: base,
				opened:    &fakeOpenedFile{Reader: bytes.NewReader([]byte("012345")), info: base},
			},
			wantClass:  resources.ErrorOversized,
			wantReason: "file_oversized",
		},
		{
			name: "symlink substituted at open",
			source: fakeSource{
				lstatInfo: base,
				openErr:   syscall.ELOOP,
			},
			wantClass: resources.ErrorSourceType,
		},
		{
			name: "inode replacement",
			source: fakeSource{
				lstatInfo: base,
				opened: &fakeOpenedFile{
					Reader: bytes.NewReader([]byte("ok")),
					info:   fakeInfo{nameValue: "target", modeValue: 0o600, sizeValue: 1, modTime: time.Unix(10, 0), sysValue: &syscall.Stat_t{Dev: 1, Ino: 2}},
				},
			},
			wantClass:  resources.ErrorSourceChanged,
			wantReason: "source_changed",
		},
		{
			name: "permission failure",
			source: fakeSource{
				lstatInfo: base,
				openErr:   fs.ErrPermission,
			},
			wantClass: resources.ErrorPermissionDenied,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs := Collect(context.Background(), Options{
				PlatformSupported: platform.Bool(true),
				Source:            tc.source,
				MaxFileBytes:      4,
				Targets:           []Target{{Name: "target", Path: "/watched", Type: "file", Hash: true}},
			})
			if len(obs) != 1 {
				t.Fatalf("obs = %+v", obs)
			}
			if tc.wantSuccess {
				if !obs[0].Success || obs[0].Fields["hash_complete"] != "true" {
					t.Fatalf("success obs = %+v", obs)
				}
				return
			}
			if obs[0].Success || obs[0].ErrorClass != tc.wantClass {
				t.Fatalf("obs = %+v, want class %s", obs, tc.wantClass)
			}
			if tc.wantReason != "" && obs[0].Fields["incomplete_reason"] != tc.wantReason {
				t.Fatalf("reason = %q, want %q in %+v", obs[0].Fields["incomplete_reason"], tc.wantReason, obs)
			}
		})
	}
}

func TestFilewatchTypeMismatchAndManifestTruncationDoNotOverwriteBaseline(t *testing.T) {
	dirInfo := fakeInfo{nameValue: "dir", modeValue: fs.ModeDir | 0o750, sizeValue: 0, modTime: time.Unix(1, 0), sysValue: &syscall.Stat_t{Dev: 1, Ino: 10}}
	fileInfo := fakeInfo{nameValue: "file", modeValue: 0o640, sizeValue: 1, modTime: time.Unix(1, 0), sysValue: &syscall.Stat_t{Dev: 1, Ino: 11}}
	obs := Collect(context.Background(), Options{
		PlatformSupported: platform.Bool(true),
		Source:            fakeSource{lstatInfo: dirInfo},
		Targets:           []Target{{Name: "target", Path: "/dir", Type: "file"}},
	})
	if len(obs) != 1 || obs[0].ErrorClass != resources.ErrorSourceType || obs[0].Fields["incomplete_reason"] != "type_mismatch" {
		t.Fatalf("type mismatch obs = %+v", obs)
	}
	obs = Collect(context.Background(), Options{
		PlatformSupported: platform.Bool(true),
		Source:            fakeSource{lstatInfo: fileInfo},
		Targets:           []Target{{Name: "target", Path: "/file", Type: "directory"}},
	})
	if len(obs) != 1 || obs[0].ErrorClass != resources.ErrorSourceType {
		t.Fatalf("reverse type mismatch obs = %+v", obs)
	}

	store := resources.NewMemoryStateStore()
	sourceComplete := fakeSource{
		lstatInfo: dirInfo,
		dirEntries: []fs.DirEntry{
			fakeDirEntry{nameValue: "c", infoValue: fakeInfo{nameValue: "c", modeValue: 0o600, sizeValue: 1, modTime: time.Unix(1, 0)}},
			fakeDirEntry{nameValue: "a", infoValue: fakeInfo{nameValue: "a", modeValue: 0o600, sizeValue: 1, modTime: time.Unix(1, 0)}},
			fakeDirEntry{nameValue: "b", infoValue: fakeInfo{nameValue: "b", modeValue: 0o600, sizeValue: 1, modTime: time.Unix(1, 0)}},
		},
	}
	target := Target{Name: "dir", Path: "/dir", Type: "directory", Manifest: true}
	obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Persist: true, State: store, Source: sourceComplete, MaxDirectoryEntries: 10, Targets: []Target{target}})
	if len(obs) != 1 || !obs[0].Success || obs[0].Fields["manifest_complete"] != "true" {
		t.Fatalf("complete manifest obs = %+v", obs)
	}
	sourceTruncated := sourceComplete
	sourceTruncated.dirEntries[2] = fakeDirEntry{nameValue: "z", infoValue: fakeInfo{nameValue: "z", modeValue: 0o600, sizeValue: 99, modTime: time.Unix(9, 0)}}
	obs = Collect(context.Background(), Options{PlatformSupported: platform.Bool(true), Persist: true, State: store, Source: sourceTruncated, MaxDirectoryEntries: 2, Targets: []Target{target}})
	if len(obs) != 1 || obs[0].Success || !obs[0].Stale || obs[0].Fields["change"] == "unchanged" {
		t.Fatalf("truncated manifest obs = %+v", obs)
	}
	raw, err := store.Get(context.Background(), "filewatch", "dir")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains([]byte(raw), []byte("manifest_truncated")) {
		t.Fatalf("truncated manifest overwrote baseline: %s", raw)
	}
}

func TestFilewatchStateWriteFailureAndCanceledContext(t *testing.T) {
	info := fakeInfo{nameValue: "target", modeValue: 0o600, sizeValue: 2, modTime: time.Unix(10, 0), sysValue: &syscall.Stat_t{Dev: 1, Ino: 1}}
	store := &writeFailStore{}
	obs := Collect(context.Background(), Options{
		PlatformSupported: platform.Bool(true),
		Persist:           true,
		State:             store,
		Source:            fakeSource{lstatInfo: info, opened: &fakeOpenedFile{Reader: bytes.NewReader([]byte("ok")), info: info}},
		Targets:           []Target{{Name: "target", Path: "/target", Type: "file", Hash: true}},
	})
	if len(obs) != 1 || obs[0].ErrorClass != resources.ErrorState {
		t.Fatalf("write failure obs = %+v", obs)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store = &writeFailStore{}
	obs = Collect(ctx, Options{
		PlatformSupported: platform.Bool(true),
		Persist:           true,
		State:             store,
		Source:            fakeSource{lstatInfo: info, opened: &fakeOpenedFile{Reader: bytes.NewReader([]byte("ok")), info: info}},
		Targets:           []Target{{Name: "target", Path: "/target", Type: "file", Hash: true}},
	})
	if len(obs) != 1 || obs[0].ErrorClass != resources.ErrorTimeout || store.writes != 0 {
		t.Fatalf("canceled obs=%+v writes=%d", obs, store.writes)
	}
}

type fakeSource struct {
	lstatInfo  fs.FileInfo
	lstatErr   error
	opened     OpenedFile
	openErr    error
	dirEntries []fs.DirEntry
	readDirErr error
}

func (s fakeSource) Lstat(name string) (fs.FileInfo, error) {
	if s.lstatErr != nil {
		return nil, s.lstatErr
	}
	return s.lstatInfo, nil
}

func (s fakeSource) OpenNoFollow(name string) (OpenedFile, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	return s.opened, nil
}

func (s fakeSource) ReadDir(name string) ([]fs.DirEntry, error) {
	if s.readDirErr != nil {
		return nil, s.readDirErr
	}
	return append([]fs.DirEntry(nil), s.dirEntries...), nil
}

type fakeOpenedFile struct {
	*bytes.Reader
	info fs.FileInfo
}

func (f *fakeOpenedFile) Close() error               { return nil }
func (f *fakeOpenedFile) Stat() (fs.FileInfo, error) { return f.info, nil }

type fakeInfo struct {
	nameValue string
	modeValue fs.FileMode
	sizeValue int64
	modTime   time.Time
	sysValue  any
}

func (f fakeInfo) Name() string       { return f.nameValue }
func (f fakeInfo) Size() int64        { return f.sizeValue }
func (f fakeInfo) Mode() fs.FileMode  { return f.modeValue }
func (f fakeInfo) ModTime() time.Time { return f.modTime }
func (f fakeInfo) IsDir() bool        { return f.modeValue.IsDir() }
func (f fakeInfo) Sys() any           { return f.sysValue }

type fakeDirEntry struct {
	nameValue string
	infoValue fs.FileInfo
}

func (d fakeDirEntry) Name() string               { return d.nameValue }
func (d fakeDirEntry) IsDir() bool                { return d.infoValue.IsDir() }
func (d fakeDirEntry) Type() fs.FileMode          { return d.infoValue.Mode().Type() }
func (d fakeDirEntry) Info() (fs.FileInfo, error) { return d.infoValue, nil }

type writeFailStore struct {
	writes int
}

func (s *writeFailStore) Get(ctx context.Context, collector string, target string) (string, error) {
	return "", storage.ErrNotFound
}

func (s *writeFailStore) Upsert(ctx context.Context, collector string, target string, status string, stateJSON string) error {
	s.writes++
	return errors.New("write failed")
}
