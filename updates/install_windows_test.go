//go:build windows

package updates

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Launch a harmless helper from a short-lived app process, using the same
// PowerShell invocation and null standard handles as the production updater.
func TestWindowsUpdateHelperSurvivesAppExit(t *testing.T) {
	if script := os.Getenv("DEPMANAGER_TEST_HELPER_SCRIPT"); script != "" {
		command := windowsUpdateCommand(script, "-AppProcessId", strconv.Itoa(os.Getpid()),
			"-ReadyFile", os.Getenv("DEPMANAGER_TEST_HELPER_READY"),
			"-FinishedFile", os.Getenv("DEPMANAGER_TEST_HELPER_FINISHED"))
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan error, 1)
		go func() { exited <- command.Wait() }()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := waitForUpdateHelper(ctx, os.Getenv("DEPMANAGER_TEST_HELPER_READY"), exited); err != nil {
			_ = command.Process.Kill()
			<-exited
			t.Fatal(err)
		}
		return // The helper must finish after this app process exits.
	}

	dir := filepath.Join(t.TempDir(), "app with spaces and 'quote'")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "helper.ps1")
	readyFile := filepath.Join(dir, "ready.txt")
	finishedFile := filepath.Join(dir, "finished.txt")
	const probe = `param([int]$AppProcessId, [string]$ReadyFile, [string]$FinishedFile)
$ErrorActionPreference = 'Stop'
[System.IO.File]::WriteAllText($ReadyFile, 'ready')
if (Get-Process -Id $AppProcessId -ErrorAction SilentlyContinue) {
  Wait-Process -Id $AppProcessId -Timeout 10 -ErrorAction SilentlyContinue
}
if (Get-Process -Id $AppProcessId -ErrorAction SilentlyContinue) { throw 'The app did not exit.' }
[System.IO.File]::WriteAllText($FinishedFile, 'finished')
`
	if err := os.WriteFile(script, []byte(probe), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	app := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWindowsUpdateHelperSurvivesAppExit$")
	app.Env = append(os.Environ(), "DEPMANAGER_TEST_HELPER_SCRIPT="+script,
		"DEPMANAGER_TEST_HELPER_READY="+readyFile, "DEPMANAGER_TEST_HELPER_FINISHED="+finishedFile)
	if output, err := app.CombinedOutput(); err != nil {
		t.Fatalf("app could not hand off the update: %v\n%s", err, output)
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if data, err := os.ReadFile(finishedFile); err == nil && string(data) == "finished" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("update helper did not continue after the app exited")
		case <-ticker.C:
		}
	}
}
