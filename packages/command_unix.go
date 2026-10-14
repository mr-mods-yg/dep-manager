//go:build !windows

package packages

import "os/exec"

func configurePackageCommand(command *exec.Cmd) {}
