//go:build linux

package resources

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func statFilesystem(path string) (rawFSStat, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return rawFSStat{}, fmt.Errorf("statfs %s failed: %w", path, err)
	}
	blockSize := uint64(stat.Bsize)
	return rawFSStat{
		Blocks:      stat.Blocks,
		BlockSize:   blockSize,
		FreeBlocks:  stat.Bfree,
		AvailBlocks: stat.Bavail,
		Files:       stat.Files,
		FreeFiles:   stat.Ffree,
		ReadOnly:    stat.Flags&unix.ST_RDONLY != 0,
		Type:        fmt.Sprintf("0x%x", stat.Type),
	}, nil
}
