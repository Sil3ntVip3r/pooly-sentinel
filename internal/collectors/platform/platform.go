package platform

import "runtime"

func Bool(value bool) *bool {
	return &value
}

func Supported(override *bool) bool {
	if override != nil {
		return *override
	}
	return runtime.GOOS == "linux"
}
