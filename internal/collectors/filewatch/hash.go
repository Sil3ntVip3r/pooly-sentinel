package filewatch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func manifestHash(entries []fs.DirEntry, maxEntries int) (string, int, bool, error) {
	if maxEntries <= 0 {
		maxEntries = 256
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return "", 0, false, fmt.Errorf("directory entry info failed")
		}
		uid, gid := owner(info)
		parts = append(parts, strings.Join([]string{
			entry.Name(),
			fileType(info),
			info.Mode().Perm().String(),
			fmt.Sprintf("%d", info.Size()),
			fmt.Sprintf("%d", info.ModTime().UTC().UnixNano()),
			fmt.Sprintf("%d", uid),
			fmt.Sprintf("%d", gid),
		}, "\t"))
	}
	sort.Strings(parts)
	observed := len(parts)
	truncated := observed > maxEntries
	if truncated {
		parts = parts[:maxEntries]
	}
	return hashBytes([]byte(strings.Join(parts, "\n"))), observed, truncated, nil
}
