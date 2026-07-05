package resources

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func openOSFile(name string) (fs.File, error) {
	return os.Open(name)
}

type MapFileSource struct {
	Files map[string]string
	Dirs  map[string][]fs.DirEntry
}

func (m MapFileSource) ReadFile(name string) ([]byte, error) {
	if m.Files == nil {
		return nil, fs.ErrNotExist
	}
	if value, ok := m.Files[cleanSourcePath(name)]; ok {
		return []byte(value), nil
	}
	return nil, fs.ErrNotExist
}

func (m MapFileSource) ReadDir(name string) ([]fs.DirEntry, error) {
	if m.Dirs == nil {
		return nil, fs.ErrNotExist
	}
	if entries, ok := m.Dirs[cleanSourcePath(name)]; ok {
		return entries, nil
	}
	return nil, fs.ErrNotExist
}

func cleanSourcePath(name string) string {
	if name == "" {
		return name
	}
	clean := filepath.Clean(name)
	if strings.HasPrefix(name, "/") && !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	return clean
}

type DirEntry struct {
	NameValue string
	Dir       bool
}

func (d DirEntry) Name() string { return d.NameValue }
func (d DirEntry) IsDir() bool  { return d.Dir }
func (d DirEntry) Type() fs.FileMode {
	if d.Dir {
		return fs.ModeDir
	}
	return 0
}
func (d DirEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrInvalid }
