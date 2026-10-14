//go:build windows

package updates

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func installAppUpdate(ctx context.Context, installer, executable string) error {
	script, err := os.CreateTemp(filepath.Dir(installer), "install-*.ps1")
	if err != nil {
		return err
	}
	handedOff := false
	defer func() {
		if !handedOff {
			_ = os.Remove(script.Name())
		}
	}()
	if _, err = script.WriteString(windowsUpdateScript); err != nil {
		script.Close()
		return err
	}
	if err = script.Close(); err != nil {
		return err
	}
	readyFile := script.Name() + ".ready"
	defer os.Remove(readyFile)
	command := windowsUpdateCommand(script.Name(),
		"-Installer", installer, "-Executable", executable, "-AppProcessId", strconv.Itoa(os.Getpid()), "-ErrorFile", updateErrorPath(), "-ReadyFile", readyFile)
	if err = command.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() {
		exited <- command.Wait()
		close(exited)
	}()
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := waitForUpdateHelper(readyCtx, readyFile, exited); err != nil {
		_ = command.Process.Kill()
		<-exited
		return err
	}
	handedOff = true
	return nil
}

func windowsUpdateCommand(script string, args ...string) *exec.Cmd {
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	command := exec.Command(powershell, append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script}, args...)...)
	const (
		createNewProcessGroup = 0x00000200
		createNoWindow        = 0x08000000
	)
	// Windows PowerShell exits without running the script with DETACHED_PROCESS
	// and null standard handles. CREATE_NO_WINDOW runs it without a console;
	// the separate process group lets the helper continue after app shutdown.
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNewProcessGroup | createNoWindow}
	return command
}
