package updates

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Run the actual handoff script with Start-Process replaced. This exercises
// PowerShell's argument binding and failure paths without launching installers.
func TestWindowsUpdateHandoff(t *testing.T) {
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil && runtime.GOOS != "windows" {
		powershell, err = exec.LookPath("pwsh")
	}
	if err != nil {
		if runtime.GOOS == "windows" {
			t.Fatal("Windows PowerShell is required to test the update helper")
		}
		t.Skip("PowerShell is not installed")
	}
	for _, test := range []struct {
		name         string
		exitCode     int
		restartErr   bool
		stillRunning bool
		missing      string
		wantError    string
	}{
		{name: "success"},
		{name: "installer failure", exitCode: 7, wantError: "code 7"},
		{name: "restart failure", restartErr: true, wantError: "Restart failed: test restart failure"},
		{name: "both failures", exitCode: 7, restartErr: true, wantError: "Restart failed: test restart failure"},
		{name: "missing installer", missing: "installer", wantError: "installer is missing"},
		{name: "missing executable", missing: "executable", wantError: "executable is missing"},
		{name: "shutdown timeout", stillRunning: true, wantError: "application did not close"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "app with spaces and 'quote'")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(dir, "update.ps1")
			installer := filepath.Join(dir, "setup.exe")
			executable := filepath.Join(dir, "depmanager.exe")
			errorFile := filepath.Join(dir, "error.txt")
			logFile := filepath.Join(dir, "calls.jsonl")
			readyFile := filepath.Join(dir, "ready.txt")
			if err := os.WriteFile(script, []byte(windowsUpdateScript), 0600); err != nil {
				t.Fatal(err)
			}
			for name, path := range map[string]string{"installer": installer, "executable": executable} {
				if name != test.missing {
					if err := os.WriteFile(path, []byte("fake"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			mock := `$env:DEPMANAGER_TEST_APP_RUNNING = 'true'
function Get-Process {
  param([int]$Id, [string]$ErrorAction)
  if ($env:DEPMANAGER_TEST_APP_RUNNING -eq 'true') { return @{Id=$Id} }
}
function Wait-Process {
  param([int]$Id, [int]$Timeout, [string]$ErrorAction)
  if ([System.IO.File]::ReadAllText($env:DEPMANAGER_TEST_READY) -ne 'ready') { throw 'Waited for shutdown before acknowledging readiness.' }
  if (Test-Path -LiteralPath $env:DEPMANAGER_TEST_LOG) { throw 'Launched a process before application shutdown.' }
  if ($env:DEPMANAGER_TEST_STILL_RUNNING -ne 'true') { $env:DEPMANAGER_TEST_APP_RUNNING = 'false' }
}
function Start-Process {
  param([string]$FilePath, [string[]]$ArgumentList, [string]$WindowStyle, [string]$WorkingDirectory, [switch]$Wait, [switch]$PassThru)
  @{path=$FilePath; arguments=$ArgumentList; style=$WindowStyle; directory=$WorkingDirectory; wait=[bool]$Wait} | ConvertTo-Json -Compress | Add-Content -LiteralPath $env:DEPMANAGER_TEST_LOG
  if ($PassThru) { return @{ExitCode=[int]$env:DEPMANAGER_TEST_EXIT} }
  if ($env:DEPMANAGER_TEST_RESTART_ERROR -eq 'true') { throw 'test restart failure' }
}
& $env:DEPMANAGER_TEST_SCRIPT -Installer $env:DEPMANAGER_TEST_INSTALLER -Executable $env:DEPMANAGER_TEST_EXE -AppProcessId 2147483647 -ErrorFile $env:DEPMANAGER_TEST_ERROR -ReadyFile $env:DEPMANAGER_TEST_READY`
			command := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", mock)
			command.Env = append(os.Environ(), "DEPMANAGER_TEST_LOG="+logFile, "DEPMANAGER_TEST_EXIT="+strconv.Itoa(test.exitCode), "DEPMANAGER_TEST_SCRIPT="+script, "DEPMANAGER_TEST_INSTALLER="+installer, "DEPMANAGER_TEST_EXE="+executable, "DEPMANAGER_TEST_ERROR="+errorFile, "DEPMANAGER_TEST_READY="+readyFile, "DEPMANAGER_TEST_RESTART_ERROR="+strconv.FormatBool(test.restartErr), "DEPMANAGER_TEST_STILL_RUNNING="+strconv.FormatBool(test.stillRunning))
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("script failed: %v\n%s", err, output)
			}
			data, err := os.ReadFile(logFile)
			if test.missing != "" || test.stillRunning {
				if !os.IsNotExist(err) {
					t.Fatalf("helper launched processes without passing preflight and completing shutdown: %s, %v", data, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				if len(lines) != 2 {
					t.Fatalf("expected install and restart calls: %s", data)
				}
				var install struct {
					Path      string
					Arguments []string
					Style     string
					Wait      bool
				}
				if err := json.Unmarshal([]byte(lines[0]), &install); err != nil {
					t.Fatal(err)
				}
				if install.Path != installer || install.Style != "Hidden" || !install.Wait || len(install.Arguments) != 2 || install.Arguments[0] != "/S" || install.Arguments[1] != "/D="+dir {
					t.Fatalf("invalid install invocation: %+v", install)
				}
				var restart struct {
					Path      string
					Style     string
					Directory string
				}
				if err := json.Unmarshal([]byte(lines[1]), &restart); err != nil {
					t.Fatal(err)
				}
				if restart.Path != executable || restart.Style != "Normal" || restart.Directory != dir {
					t.Fatalf("invalid restart: %+v", restart)
				}
			}
			errorData, err := os.ReadFile(errorFile)
			if test.wantError == "" && !os.IsNotExist(err) {
				t.Fatalf("unexpected error file: %s, %v", errorData, err)
			}
			if test.wantError != "" && (err != nil || !strings.Contains(string(errorData), test.wantError)) {
				t.Fatalf("failure was not recorded: %s, %v", errorData, err)
			}
			if test.exitCode != 0 && !strings.Contains(string(errorData), "code 7") {
				t.Fatalf("installer error was lost: %s", errorData)
			}
			readyData, err := os.ReadFile(readyFile)
			if err != nil {
				t.Fatal(err)
			}
			if test.missing == "" && string(readyData) != "ready" {
				t.Fatalf("helper did not acknowledge readiness: %s", readyData)
			}
			if test.missing != "" && !strings.Contains(string(readyData), "error: ") {
				t.Fatalf("helper did not report preflight failure: %s", readyData)
			}
			for _, path := range []string{script, installer} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("helper did not clean up %s: %v", path, err)
				}
			}
		})
	}
}

func TestUpdateHelperReadiness(t *testing.T) {
	for _, test := range []struct {
		name, status, wantError string
		exited                  bool
		exitErr                 error
	}{
		{name: "ready", status: "ready"},
		{name: "preflight failure", status: "error: installer is missing", wantError: "installer is missing"},
		{name: "early exit", exited: true, wantError: "exited before it was ready"},
		{name: "failed exit", exited: true, exitErr: errors.New("exit status 1"), wantError: "exit status 1"},
		{name: "timeout", wantError: "deadline exceeded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			readyFile := filepath.Join(t.TempDir(), "ready")
			if test.status != "" {
				if err := os.WriteFile(readyFile, []byte(test.status), 0600); err != nil {
					t.Fatal(err)
				}
			}
			exited := make(chan error, 1)
			if test.exited {
				exited <- test.exitErr
				close(exited)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			err := waitForUpdateHelper(ctx, readyFile, exited)
			if test.wantError == "" && err != nil || test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError)) {
				t.Fatalf("got %v, want %q", err, test.wantError)
			}
		})
	}
}
