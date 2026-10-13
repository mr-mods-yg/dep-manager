//go:build windows

package updates

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

// Run the existing per-user NSIS installer after the webview has released
// its executable. Parameters are passed as arguments, never interpolated.
const windowsUpdateScript = `param(
  [string]$Installer, [string]$Executable, [int]$AppProcessId, [string]$ErrorFile
)
$ErrorActionPreference = 'Stop'
try {
  if (Get-Process -Id $AppProcessId -ErrorAction SilentlyContinue) {
    Wait-Process -Id $AppProcessId -Timeout 60 -ErrorAction SilentlyContinue
  }
  if (Get-Process -Id $AppProcessId -ErrorAction SilentlyContinue) {
    throw 'The application did not close. Please retry the update.'
  }
  $installDir = [System.IO.Path]::GetDirectoryName($Executable)
  $result = Start-Process -FilePath $Installer -ArgumentList @('/S', "/D=$installDir") -WindowStyle Hidden -Wait -PassThru
  if ($result.ExitCode -ne 0) { throw "Installer exited with code $($result.ExitCode)." }
} catch {
  $_.Exception.Message | Set-Content -LiteralPath $ErrorFile -Encoding UTF8
} finally {
  if (-not (Get-Process -Id $AppProcessId -ErrorAction SilentlyContinue)) {
    Start-Process -FilePath $Executable -WindowStyle Hidden
  }
  Remove-Item -LiteralPath $Installer -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
}
`

func installAppUpdate(ctx context.Context, installer, executable string) error {
	script, err := os.CreateTemp(filepath.Dir(installer), "install-*.ps1")
	if err != nil {
		return err
	}
	if _, err = script.WriteString(windowsUpdateScript); err != nil {
		script.Close()
		return err
	}
	if err = script.Close(); err != nil {
		return err
	}
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	command := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script.Name(),
		"-Installer", installer, "-Executable", executable, "-AppProcessId", strconv.Itoa(os.Getpid()), "-ErrorFile", updateErrorPath())
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200 | 0x00000008}
	if err = command.Start(); err != nil {
		_ = os.Remove(script.Name())
		return err
	}
	return command.Process.Release()
}
