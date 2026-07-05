//go:build !unix

package filewatch

import "os"

func openNoFollow(name string) (OpenedFile, error) {
	return os.Open(name)
}
