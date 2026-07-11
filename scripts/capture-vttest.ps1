param(
  [Parameter(Mandatory=$true)][string]$VttestExe,
  [string]$Output = "internal\vt\testdata\vttest-manual.vt",
  [int]$Rows = 24,
  [int]$Cols = 80,
  [string]$CervTermExe = "dist\cervterm-capture.exe",
  [string]$Msys2Bash = "C:\msys64\usr\bin\bash.exe",
  [string]$Timeout = "30s"
)

$ErrorActionPreference = "Stop"
if (-not (Test-Path $VttestExe)) {
  throw "vttest executable not found: $VttestExe"
}
if (-not (Test-Path $CervTermExe)) {
  go build -o $CervTermExe ./cmd/cervterm
}
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Output) | Out-Null

if (Test-Path $Msys2Bash) {
  $resolvedVttest = (Resolve-Path $VttestExe).Path -replace '\\', '/'
  if ($resolvedVttest -match '^([A-Za-z]):/(.*)$') {
    $drive = $matches[1].ToLower()
    $rest = $matches[2]
    $resolvedVttest = "/$drive/$rest"
  }
  & $CervTermExe --capture-vt $Output --capture-program $Msys2Bash --capture-arg "-lc" --capture-arg "TERM=xterm '$resolvedVttest'" --capture-rows $Rows --capture-cols $Cols --capture-timeout $Timeout
} else {
  & $CervTermExe --capture-vt $Output --capture-program $VttestExe --capture-rows $Rows --capture-cols $Cols --capture-timeout $Timeout
}
Write-Host "Captured $Output"
