//go:build unix

package filewatch

import (
	"os"
	"syscall"
)

func openNoFollow(name string) (OpenedFile, error) {
	fd, err := syscall.Open(name, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
