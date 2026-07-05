package ssh

import (
	"fmt"
	"strings"
)

type ExpectedConfig struct {
	Ports                        []int
	ForbiddenPorts               []int
	PermitRootLogin              string
	PasswordAuthentication       string
	KbdInteractiveAuthentication string
	PermitEmptyPasswords         string
	PubkeyAuthentication         string
	StrictModes                  string
	PermitUserEnvironment        string
}

var expectedDirectives = []string{
	"permitrootlogin",
	"passwordauthentication",
	"kbdinteractiveauthentication",
	"permitemptypasswords",
	"pubkeyauthentication",
	"strictmodes",
	"permituserenvironment",
}

func ParseEffectiveConfig(output string) (map[string]string, error) {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("sshd effective config line is malformed")
		}
		key := strings.ToLower(fields[0])
		if !isSafeDirective(key) {
			continue
		}
		values[key] = strings.ToLower(strings.Join(fields[1:], " "))
	}
	return values, nil
}

func expectedDirectiveMap(expected ExpectedConfig) map[string]string {
	return map[string]string{
		"permitrootlogin":              strings.ToLower(expected.PermitRootLogin),
		"passwordauthentication":       strings.ToLower(expected.PasswordAuthentication),
		"kbdinteractiveauthentication": strings.ToLower(expected.KbdInteractiveAuthentication),
		"permitemptypasswords":         strings.ToLower(expected.PermitEmptyPasswords),
		"pubkeyauthentication":         strings.ToLower(expected.PubkeyAuthentication),
		"strictmodes":                  strings.ToLower(expected.StrictModes),
		"permituserenvironment":        strings.ToLower(expected.PermitUserEnvironment),
	}
}

func isSafeDirective(key string) bool {
	for _, allowed := range expectedDirectives {
		if key == allowed {
			return true
		}
	}
	return false
}
