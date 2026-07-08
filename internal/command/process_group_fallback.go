//go:build !unix

package command

import (
	"os"
	"os/exec"
)

func configureCommandProcess(*exec.Cmd) {}

func terminateCommandProcessGroup(*exec.Cmd) error {
	return os.ErrProcessDone
}
