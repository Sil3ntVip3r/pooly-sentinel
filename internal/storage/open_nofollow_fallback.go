//go:build !unix

package storage

import (
	"fmt"
	"os"
)

func openRegularNoFollow(name string, flag int, perm os.FileMode) (*os.File, error) {
	info, err := os.Lstat(name)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("path is a symlink")
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("path is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	info, err = file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("path is not a regular file")
	}
	return file, nil
}
