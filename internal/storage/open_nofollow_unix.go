//go:build unix

package storage

import (
	"fmt"
	"os"
	"syscall"
)

func openRegularNoFollow(name string, flag int, perm os.FileMode) (*os.File, error) {
	fd, err := syscall.Open(name, flag|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, uint32(perm.Perm()))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	info, err := file.Stat()
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
