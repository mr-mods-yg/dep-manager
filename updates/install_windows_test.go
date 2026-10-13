//go:build windows

package updates

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Run the actual handoff script with Start-Process replaced. This exercises
// PowerShell's argument binding and failure paths without launching installers.
func TestWindowsUpdateHandoff(t *testing.T) {
	for _, exitCode := range []int{0, 7} {
		t.Run(strconv.Itoa(exitCode), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "app with spaces and 'quote'")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(dir, "update.ps1")
			installer := filepath.Join(dir, "setup.exe")
			executable := filepath.Join(dir, "depmanager.exe")
			errorFile := filepath.Join(dir, "error.txt")
			logFile := filepath.Join(dir, "calls.jsonl")
			if err := os.WriteFile(script, []byte(windowsUpdateScript), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(installer, []byte("fake"), 0600); err != nil {
				t.Fatal(err)
			}
			mock := `function Start-Process {
  param([string]$FilePath, [string[]]$ArgumentList, [string]$WindowStyle, [switch]$Wait, [switch]$PassThru)
  @{path=$FilePath; arguments=$ArgumentList; style=$WindowStyle; wait=[bool]$Wait} | ConvertTo-Json -Compress | Add-Content -LiteralPath $env:DEPMANAGER_TEST_LOG
  if ($PassThru) { return @{ExitCode=[int]$env:DEPMANAGER_TEST_EXIT} }
}
& $env:DEPMANAGER_TEST_SCRIPT -Installer $env:DEPMANAGER_TEST_INSTALLER -Executable $env:DEPMANAGER_TEST_EXE -AppProcessId 2147483647 -ErrorFile $env:DEPMANAGER_TEST_ERROR`
			command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", mock)
			command.Env = append(os.Environ(), "DEPMANAGER_TEST_LOG="+logFile, "DEPMANAGER_TEST_EXIT="+strconv.Itoa(exitCode), "DEPMANAGER_TEST_SCRIPT="+script, "DEPMANAGER_TEST_INSTALLER="+installer, "DEPMANAGER_TEST_EXE="+executable, "DEPMANAGER_TEST_ERROR="+errorFile)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("script failed: %v\n%s", err, output)
			}
			data, err := os.ReadFile(logFile)
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
				Path  string
				Style string
			}
			if err := json.Unmarshal([]byte(lines[1]), &restart); err != nil {
				t.Fatal(err)
			}
			if restart.Path != executable || restart.Style != "Hidden" {
				t.Fatalf("invalid restart: %+v", restart)
			}
			errorData, err := os.ReadFile(errorFile)
			if exitCode == 0 && !os.IsNotExist(err) {
				t.Fatalf("unexpected error file: %s, %v", errorData, err)
			}
			if exitCode != 0 && (err != nil || !strings.Contains(string(errorData), "code 7")) {
				t.Fatalf("failure was not recorded: %s, %v", errorData, err)
			}
		})
	}
}
