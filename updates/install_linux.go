//go:build linux

package updates

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func installAppUpdate(ctx context.Context, installer, executable string) error {
	dpkg, err := exec.LookPath("dpkg")
	if err != nil {
		return fmt.Errorf("automatic updates require a Debian/Ubuntu installation: %w", err)
	}
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return fmt.Errorf("install polkit (pkexec) to authorize application updates: %w", err)
	}
	command := exec.CommandContext(ctx, pkexec, dpkg, "--install", installer)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("installation was cancelled or failed: %w: %s", err, truncate(strings.TrimSpace(string(output)), 1000))
	}
	// Restart as the original user, never as the privileged installer user.
	restart := exec.Command("/bin/sh", "-c", `remaining=60; while kill -0 "$1" 2>/dev/null; do
  remaining=$((remaining - 1)); [ "$remaining" -gt 0 ] || exit 1; sleep 1
done
exec "$2"`, "depmanager-restart", strconv.Itoa(os.Getpid()), executable)
	restart.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := restart.Start(); err != nil {
		return err
	}
	_ = os.Remove(installer)
	return restart.Process.Release()
}
