param(
  [string]$Arch = "amd64",
  [string]$Tool = "goversioninfo"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$src = Join-Path $root "packaging\windows"
$outName = "resource_windows_$Arch.syso"
$outPath = Join-Path $root "cmd\cervterm\$outName"
$tmp = Join-Path $root "dist\resource-$Arch"

if (-not (Get-Command $Tool -ErrorAction SilentlyContinue)) {
  throw "goversioninfo not found. Install with: go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest"
}

if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp }
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
Copy-Item (Join-Path $src "versioninfo.json") $tmp
Copy-Item (Join-Path $src "cervterm.ico") $tmp
Copy-Item (Join-Path $src "cervterm.manifest") $tmp

Push-Location $tmp
try {
  & $Tool -platform-specific -64
  if (-not (Test-Path $outName)) {
    throw "goversioninfo did not produce $outName"
  }
  Copy-Item $outName $outPath -Force
  Write-Host "Wrote $outPath"
} finally {
  Pop-Location
}
