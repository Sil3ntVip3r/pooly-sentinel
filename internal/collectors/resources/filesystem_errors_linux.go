//go:build linux

package resources

import "errors"

var errFilesystemUnsupported = errors.New("unsupported platform")
