//go:build windows

package packages

import (
	"os/exec"
	"syscall"
)

func configurePackageCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
