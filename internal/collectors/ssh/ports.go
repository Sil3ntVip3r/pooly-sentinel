package ssh

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseListeningPorts(output string) (map[int]bool, error) {
	ports := map[int]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			return nil, fmt.Errorf("ss listener line is malformed")
		}
		if strings.ToUpper(fields[0]) != "LISTEN" {
			continue
		}
		local := fields[3]
		port, ok := parsePortFromAddress(local)
		if ok {
			ports[port] = true
		}
	}
	return ports, nil
}

func parsePortFromAddress(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if strings.HasPrefix(value, "[") {
		end := strings.LastIndex(value, "]:")
		if end >= 0 {
			return parsePort(value[end+2:])
		}
	}
	idx := strings.LastIndex(value, ":")
	if idx < 0 || idx+1 >= len(value) {
		return 0, false
	}
	return parsePort(value[idx+1:])
}

func parsePort(value string) (int, bool) {
	value = strings.Trim(value, "*")
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}
