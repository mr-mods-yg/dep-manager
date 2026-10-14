package updates

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Run the per-user NSIS installer after the app releases its executable.
// Parameters are passed as arguments, never interpolated into script source.
const windowsUpdateScript = `param(
  [string]$Installer, [string]$Executable, [int]$AppProcessId,
  [string]$ErrorFile, [string]$ReadyFile
)
$ErrorActionPreference = 'Stop'
$ready = $false
try {
  if (-not (Test-Path -LiteralPath $Installer -PathType Leaf)) { throw 'The downloaded installer is missing.' }
  if (-not (Test-Path -LiteralPath $Executable -PathType Leaf)) { throw 'The application executable is missing.' }
  $installDir = [System.IO.Path]::GetDirectoryName($Executable)
  [System.IO.File]::WriteAllText($ReadyFile, 'ready')
  $ready = $true
  if (Get-Process -Id $AppProcessId -ErrorAction SilentlyContinue) {
    Wait-Process -Id $AppProcessId -Timeout 60 -ErrorAction SilentlyContinue
  }
  if (Get-Process -Id $AppProcessId -ErrorAction SilentlyContinue) {
    throw 'The application did not close. Please retry the update.'
  }
  $result = Start-Process -FilePath $Installer -ArgumentList @('/S', "/D=$installDir") -WindowStyle Hidden -Wait -PassThru
  if ($result.ExitCode -ne 0) { throw "Installer exited with code $($result.ExitCode)." }
} catch {
  [System.IO.File]::WriteAllText($ErrorFile, $_.Exception.Message)
  if (-not $ready) { [System.IO.File]::WriteAllText($ReadyFile, "error: $($_.Exception.Message)") }
} finally {
  try {
    if ($ready -and -not (Get-Process -Id $AppProcessId -ErrorAction SilentlyContinue)) {
      Start-Process -FilePath $Executable -WorkingDirectory $installDir -WindowStyle Normal
    }
  } catch {
    [System.IO.File]::AppendAllText($ErrorFile, [Environment]::NewLine + "Restart failed: $($_.Exception.Message)")
  } finally {
    Remove-Item -LiteralPath $Installer -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
  }
}
`

// A spawned PowerShell process can fail before executing the script (for
// example, a policy can block it). Keep the app open until the helper is ready.
func waitForUpdateHelper(ctx context.Context, readyFile string, exited <-chan error) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(readyFile)
		if err == nil {
			status := strings.TrimSpace(string(data))
			if status == "ready" {
				return nil
			}
			if strings.HasPrefix(status, "error: ") {
				return fmt.Errorf("update helper could not start: %s", strings.TrimPrefix(status, "error: "))
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("could not read update helper status: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("update helper did not become ready: %w", ctx.Err())
		case err := <-exited:
			if err != nil {
				return fmt.Errorf("update helper exited before it was ready: %w", err)
			}
			return fmt.Errorf("update helper exited before it was ready")
		case <-ticker.C:
		}
	}
}
