//go:build !windows

package main

import "os/exec"

func configurePackageCommand(command *exec.Cmd) {}
