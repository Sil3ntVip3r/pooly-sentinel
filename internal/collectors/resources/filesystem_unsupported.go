//go:build !linux

package resources

import "errors"

var errFilesystemUnsupported = errors.New("unsupported platform")

func statFilesystem(path string) (rawFSStat, error) {
	return rawFSStat{}, errFilesystemUnsupported
}
