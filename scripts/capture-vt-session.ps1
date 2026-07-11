param(
  [Parameter(Mandatory=$true)][string]$Output,
  [string]$Command = "vttest"
)

$ErrorActionPreference = "Stop"
$dir = Split-Path -Parent $Output
if ($dir -and -not (Test-Path $dir)) {
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
}

Write-Host "Starting $Command. Transcript bytes will be written to $Output"
Write-Host "Close the child program normally when the target test screen is complete."

# PowerShell transcripts are text-oriented, so use ConPTY/script tools when
# available. This fallback captures stdout/stderr bytes for simple deterministic
# commands and documents the intended artifact path for manual vttest captures.
& $Command 2>&1 | Tee-Object -FilePath $Output | Out-Host
