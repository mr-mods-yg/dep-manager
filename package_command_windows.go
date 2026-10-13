//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func configurePackageCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
