package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ensureDir(path string) error {
	if path == "" {
		return fmt.Errorf("directory path is required")
	}
	if err := os.MkdirAll(path, DirMode); err != nil {
		return err
	}
	return nil
}

func secureJoin(base string, parts ...string) (string, error) {
	if base == "" {
		return "", fmt.Errorf("base path is required")
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("base path must be absolute")
	}
	cleanBase := filepath.Clean(base)
	joined := filepath.Join(append([]string{cleanBase}, parts...)...)
	rel, err := filepath.Rel(cleanBase, joined)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return joined, nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes base directory")
	}
	return joined, nil
}

func validatePlainFilename(name string) error {
	if name == "" {
		return fmt.Errorf("filename is required")
	}
	if filepath.IsAbs(name) || filepath.Base(name) != name || name == "." || name == ".." {
		return fmt.Errorf("filename must be a plain relative filename")
	}
	return nil
}

func sanitizeSegment(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("value is required")
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_', r == ':':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	out = strings.TrimSpace(out)
	if out == "" || out == "." || out == ".." {
		return "", fmt.Errorf("value does not contain a safe path segment")
	}
	return out, nil
}

func ensureDirNoSymlink(path string) error {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, volume)
	absolute := strings.HasPrefix(rest, string(filepath.Separator))
	parts := strings.Split(strings.Trim(rest, string(filepath.Separator)), string(filepath.Separator))
	current := volume
	if absolute {
		current += string(filepath.Separator)
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path contains symlink: %s", current)
			}
			if !info.IsDir() {
				return fmt.Errorf("path is not a directory: %s", current)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.Mkdir(current, DirMode); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func ensureDirNoSymlinkUnder(base string, path string) error {
	if base == "" || path == "" {
		return fmt.Errorf("base and path are required")
	}
	cleanBase := filepath.Clean(base)
	cleanPath := filepath.Clean(path)
	if err := ensureDir(cleanBase); err != nil {
		return err
	}
	baseInfo, err := os.Lstat(cleanBase)
	if err != nil {
		return err
	}
	if baseInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("configured root is a symlink: %s", cleanBase)
	}
	rel, err := filepath.Rel(cleanBase, cleanPath)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes base directory")
	}
	current := cleanBase
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path contains symlink: %s", current)
			}
			if !info.IsDir() {
				return fmt.Errorf("path is not a directory: %s", current)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.Mkdir(current, DirMode); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}
