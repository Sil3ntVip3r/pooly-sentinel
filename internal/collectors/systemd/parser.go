package systemd

import (
	"fmt"
	"strconv"
	"strings"
)

type UnitState struct {
	Name                          string
	LoadState                     string
	ActiveState                   string
	SubState                      string
	UnitFileState                 string
	Result                        string
	MainPID                       int64
	ExecMainCode                  int64
	ExecMainStatus                int64
	NRestarts                     int64
	ActiveEnterTimestampMonotonic int64
}

var allowedProperties = []string{
	"Id",
	"LoadState",
	"ActiveState",
	"SubState",
	"UnitFileState",
	"Result",
	"MainPID",
	"ExecMainCode",
	"ExecMainStatus",
	"NRestarts",
	"ActiveEnterTimestampMonotonic",
}

func propertyList() string {
	return strings.Join(allowedProperties, ",")
}

func ParseShowOutput(unit string, output string) (UnitState, error) {
	state := UnitState{Name: normalizeUnitName(unit)}
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return UnitState{}, fmt.Errorf("systemctl show line is malformed")
		}
		if !isAllowedProperty(key) {
			continue
		}
		seen[key] = true
		switch key {
		case "Id":
			if value != "" {
				state.Name = normalizeUnitName(value)
			}
		case "LoadState":
			state.LoadState = safeStateValue(value)
		case "ActiveState":
			state.ActiveState = safeStateValue(value)
		case "SubState":
			state.SubState = safeStateValue(value)
		case "UnitFileState":
			state.UnitFileState = safeStateValue(value)
		case "Result":
			state.Result = safeStateValue(value)
		case "MainPID":
			parsed, err := parseNonNegativeIntProperty(value)
			if err != nil {
				return UnitState{}, fmt.Errorf("MainPID is malformed")
			}
			state.MainPID = parsed
		case "ExecMainCode":
			parsed, err := parseIntProperty(value)
			if err != nil {
				return UnitState{}, fmt.Errorf("ExecMainCode is malformed")
			}
			state.ExecMainCode = parsed
		case "ExecMainStatus":
			parsed, err := parseIntProperty(value)
			if err != nil {
				return UnitState{}, fmt.Errorf("ExecMainStatus is malformed")
			}
			state.ExecMainStatus = parsed
		case "NRestarts":
			parsed, err := parseNonNegativeIntProperty(value)
			if err != nil {
				return UnitState{}, fmt.Errorf("NRestarts is malformed")
			}
			state.NRestarts = parsed
		case "ActiveEnterTimestampMonotonic":
			parsed, err := parseNonNegativeIntProperty(value)
			if err != nil {
				return UnitState{}, fmt.Errorf("ActiveEnterTimestampMonotonic is malformed")
			}
			state.ActiveEnterTimestampMonotonic = parsed
		}
	}
	for _, required := range []string{"LoadState", "ActiveState", "SubState"} {
		if !seen[required] {
			return UnitState{}, fmt.Errorf("systemctl show output missing %s", required)
		}
	}
	return state, nil
}

func isAllowedProperty(key string) bool {
	for _, allowed := range allowedProperties {
		if key == allowed {
			return true
		}
	}
	return false
}

func safeStateValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > 64 {
		return value[:64]
	}
	return value
}

func parseIntProperty(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func parseNonNegativeIntProperty(value string) (int64, error) {
	parsed, err := parseIntProperty(value)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, fmt.Errorf("must not be negative")
	}
	return parsed, nil
}

func normalizeUnitName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown.service"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == '@' || r == ':' {
			b.WriteRune(r)
		}
		if b.Len() >= 128 {
			break
		}
	}
	if b.Len() == 0 {
		return "unknown.service"
	}
	return b.String()
}
