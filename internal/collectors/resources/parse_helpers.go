package resources

import (
	"fmt"
	"strconv"
	"strings"
)

func parseUintText(data []byte, name string) (uint64, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is malformed", name)
	}
	return value, nil
}

func readUintFile(source FileSource, name string) (uint64, error) {
	data, err := source.ReadFile(name)
	if err != nil {
		return 0, err
	}
	return parseUintText(data, name)
}
