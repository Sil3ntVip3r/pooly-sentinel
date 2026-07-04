package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var shellNames = map[string]struct{}{
	"sh":         {},
	"bash":       {},
	"zsh":        {},
	"dash":       {},
	"fish":       {},
	"cmd":        {},
	"cmd.exe":    {},
	"powershell": {},
	"pwsh":       {},
}

func ValidateSpec(spec CommandSpec) error {
	if spec.Path == "" {
		return fmt.Errorf("command path is required")
	}
	if strings.ContainsAny(spec.Path, "\x00\n\r\t|&;<>(){}$`") || strings.Contains(spec.Path, " ") {
		return fmt.Errorf("command path must be a single executable path, not a shell command string")
	}
	if _, isShell := shellNames[filepath.Base(spec.Path)]; isShell {
		return fmt.Errorf("shell executables are not allowed")
	}
	if spec.MaxStdout <= 0 {
		return fmt.Errorf("max stdout must be greater than zero")
	}
	if spec.MaxStderr <= 0 {
		return fmt.Errorf("max stderr must be greater than zero")
	}
	if spec.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}
	return nil
}

func resolveExecutable(path string) (string, error) {
	if filepath.IsAbs(path) || strings.ContainsRune(path, os.PathSeparator) {
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		return path, nil
	}
	return exec.LookPath(path)
}
