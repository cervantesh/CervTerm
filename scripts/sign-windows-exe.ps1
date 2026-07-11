param(
  [Parameter(Mandatory=$true)][string]$ExePath,
  [Parameter(Mandatory=$true)][string]$PfxBase64,
  [Parameter(Mandatory=$true)][string]$PfxPassword,
  [string]$TimestampUrl = "http://timestamp.digicert.com"
)

$ErrorActionPreference = "Stop"
if (-not (Test-Path $ExePath)) {
  throw "Executable not found: $ExePath"
}

$signtool = Get-Command signtool.exe -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Source
if (-not $signtool) {
  $kits = Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"
  if (Test-Path $kits) {
    $signtool = Get-ChildItem $kits -Filter signtool.exe -Recurse | Where-Object { $_.FullName -match '\\x64\\signtool\.exe$' } | Sort-Object FullName -Descending | Select-Object -First 1 -ExpandProperty FullName
  }
}
if (-not $signtool) {
  throw "signtool.exe not found. Install Windows SDK or run on windows-latest."
}

$pfx = Join-Path $env:RUNNER_TEMP "cervterm-codesign.pfx"
[IO.File]::WriteAllBytes($pfx, [Convert]::FromBase64String($PfxBase64))
try {
  & $signtool sign /fd SHA256 /tr $TimestampUrl /td SHA256 /f $pfx /p $PfxPassword $ExePath
  & $signtool verify /pa /v $ExePath
  Write-Host "Signed $ExePath"
} finally {
  Remove-Item -Force $pfx -ErrorAction SilentlyContinue
}
