package resources

import (
	"path"
	"path/filepath"
	"runtime"
)

func pathMatch(pattern string, name string) (bool, error) {
	if runtime.GOOS == "windows" {
		return path.Match(filepath.ToSlash(pattern), filepath.ToSlash(name))
	}
	return path.Match(pattern, name)
}
