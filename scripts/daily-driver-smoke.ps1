param(
  [string]$CervTermExe = "",
  [string]$WorkDir = "dist/daily-driver-smoke",
  [switch]$BuildIfMissing,
  [string]$Version = "daily-smoke"
)

$ErrorActionPreference = "Stop"

function Resolve-CervTermExe {
  if ($CervTermExe) {
    $resolved = Resolve-Path $CervTermExe
    return $resolved.Path
  }
  $candidate = Join-Path $WorkDir "cervterm.exe"
  if ((Test-Path $candidate) -and -not $BuildIfMissing.IsPresent) {
    return (Resolve-Path $candidate).Path
  }
  New-Item -ItemType Directory -Force -Path $WorkDir | Out-Null
  go build -tags glfw -ldflags "-X cervterm/internal/buildinfo.Version=$Version" -o $candidate ./cmd/cervterm
  return (Resolve-Path $candidate).Path
}

function Assert-FileContains {
  param(
    [Parameter(Mandatory = $true)] [string]$Path,
    [Parameter(Mandatory = $true)] [string]$Pattern,
    [Parameter(Mandatory = $true)] [string]$CaseName
  )
  if (-not (Test-Path $Path)) { throw "$CaseName did not create $Path" }
  if ((Get-Item $Path).Length -le 0) { throw "$CaseName capture is empty: $Path" }
  $text = Get-Content -Raw -ErrorAction Stop $Path
  if ($text -notmatch [regex]::Escape($Pattern)) {
    throw "$CaseName capture missing marker '$Pattern' in $Path"
  }
}

function New-PowerShellScriptArgs {
  param(
    [Parameter(Mandatory = $true)] [string]$Name,
    [Parameter(Mandatory = $true)] [string]$Command
  )
  $powershellExe = Join-Path $env:SystemRoot "System32/WindowsPowerShell/v1.0/powershell.exe"
  $ps1Path = Join-Path $WorkDir "$Name.ps1"
  $cmdPath = Join-Path $WorkDir "$Name.cmd"
  $Command | Set-Content -Encoding ASCII $ps1Path
  @(
    "@echo off",
    "$powershellExe -NoProfile -ExecutionPolicy Bypass -File $ps1Path"
  ) | Set-Content -Encoding ASCII $cmdPath
  return @("/C", $cmdPath)
}
function Invoke-Capture {
  param(
    [Parameter(Mandatory = $true)] [string]$Name,
    [Parameter(Mandatory = $true)] [string]$Program,
    [Parameter(Mandatory = $true)] [string[]]$Args,
    [Parameter(Mandatory = $true)] [string[]]$Markers,
    [int]$Rows = 24,
    [int]$Cols = 80,
    [string]$Timeout = "15s"
  )

  $vtPath = Join-Path $WorkDir "$Name.vt"
  $logPath = Join-Path $WorkDir "$Name.log"
  Remove-Item -Force $vtPath, $logPath -ErrorAction SilentlyContinue

  $captureArgs = @(
    "--capture-vt", $vtPath,
    "--capture-program", $Program,
    "--capture-timeout", $Timeout,
    "--capture-rows", [string]$Rows,
    "--capture-cols", [string]$Cols,
    "--log-file", $logPath
  )
  foreach ($arg in $Args) {
    $captureArgs += @("--capture-arg", $arg)
  }

  Write-Host "daily-driver: $Name ($Program $($Args -join ' '))"
  & $script:CervTermResolved @captureArgs
  if ($LASTEXITCODE -ne 0) { Write-Warning "$Name capture exited with code $LASTEXITCODE; validating captured markers before failing" }

  foreach ($marker in $Markers) {
    Assert-FileContains -Path $vtPath -Pattern $marker -CaseName $Name
  }
  if (-not (Test-Path $logPath)) { throw "$Name did not create diagnostics log $logPath" }
  Write-Host "daily-driver: $Name ok -> $vtPath"
}

if (Test-Path $WorkDir) { Remove-Item -Recurse -Force $WorkDir }
New-Item -ItemType Directory -Force -Path $WorkDir | Out-Null
$script:CervTermResolved = Resolve-CervTermExe
Write-Host "daily-driver: using $script:CervTermResolved"

$doctor = (& $script:CervTermResolved --doctor) -join "`n"
if ($doctor -notmatch "CervTerm doctor") { throw "doctor precheck failed" }

Invoke-Capture `
  -Name "cmd-basic" `
  -Program "cmd.exe" `
  -Args @("/C", "echo CERVTERM_CMD_START && ver && echo CERVTERM_CMD_END") `
  -Markers @("CERVTERM_CMD_START", "CERVTERM_CMD_END")

$powershellBasicCommand = "Write-Output 'CERVTERM_PS_START'; Get-Location; 1..3 | ForEach-Object { Write-Output ('ps-line-{0}' -f `$_) }; Write-Output 'CERVTERM_PS_END'"
Invoke-Capture `
  -Name "powershell-basic" `
  -Program "cmd.exe" `
  -Args (New-PowerShellScriptArgs -Name "powershell-basic" -Command $powershellBasicCommand) `
  -Markers @("CERVTERM_PS_START", "CERVTERM_PS_END")

$gitCommand = Get-Command git.exe -ErrorAction SilentlyContinue
if (-not $gitCommand) { throw "git.exe is required for daily-driver git-log smoke" }
$gitExe = $gitCommand.Source
$gitRoot = Split-Path (Split-Path (Split-Path $gitExe -Parent) -Parent) -Parent
$gitCmdWrapper = Join-Path $gitRoot "cmd/git.exe"
if (Test-Path $gitCmdWrapper) { $gitExe = (Resolve-Path $gitCmdWrapper).Path }
$gitHead = (& $gitExe rev-parse --short HEAD).Trim()
if (-not $gitHead) { throw "git-log smoke could not resolve HEAD" }
$gitArgs = @("--no-pager", "log", "--oneline", "-n", "3")

Invoke-Capture `
  -Name "git-log" `
  -Program $gitExe `
  -Args $gitArgs `
  -Markers @($gitHead)

$moreExe = Join-Path $env:SystemRoot "System32/more.com"
if (-not (Test-Path $moreExe)) { throw "more.com is required for pager smoke: $moreExe" }
$pagerInput = Join-Path $WorkDir "pager-input.txt"
@("CERVTERM_PAGER_START") + (1..80 | ForEach-Object { "pager-line-{0:D3} abcdefghijklmnopqrstuvwxyz" -f $_ }) + @("CERVTERM_PAGER_END") | Set-Content -Encoding ASCII $pagerInput
Invoke-Capture `
  -Name "pager-more" `
  -Program "cmd.exe" `
  -Args @("/C", "`"$moreExe`" < `"$pagerInput`"") `
  -Markers @("CERVTERM_PAGER_START", "pager-line-020", "-- More") `
  -Timeout "8s"

$altScreenCommand = '$esc=[char]27; Write-Output "CERVTERM_ALT_START"; Write-Output ($esc + ''[?1049hALT_SCREEN_BODY'' + $esc + ''[2J'' + $esc + ''[Hinside-alt-screen'' + $esc + ''[?1049l''); Write-Output "CERVTERM_ALT_END"'
Invoke-Capture `
  -Name "alternate-screen" `
  -Program "cmd.exe" `
  -Args (New-PowerShellScriptArgs -Name "alternate-screen" -Command $altScreenCommand) `
  -Markers @("CERVTERM_ALT_START", "inside-alt-screen", "CERVTERM_ALT_END")

$longLine = "CERVTERM_REFLOW_START " + ((1..12 | ForEach-Object { "segment$_" }) -join "-") + " CERVTERM_REFLOW_END"
$reflowCommand = "Write-Output '$longLine'"
Invoke-Capture `
  -Name "resize-reflow-40col" `
  -Program "cmd.exe" `
  -Args (New-PowerShellScriptArgs -Name "resize-reflow-40col" -Command $reflowCommand) `
  -Markers @("CERVTERM_REFLOW_START", "CERVTERM_REFLOW_END") `
  -Cols 40
Invoke-Capture `
  -Name "resize-reflow-100col" `
  -Program "cmd.exe" `
  -Args (New-PowerShellScriptArgs -Name "resize-reflow-100col" -Command $reflowCommand) `
  -Markers @("CERVTERM_REFLOW_START", "CERVTERM_REFLOW_END") `
  -Cols 100

$longSessionCommand = "Write-Output 'CERVTERM_LONG_START'; 1..20 | ForEach-Object { Write-Output ('long-session-line-{0:D2}' -f `$_); Start-Sleep -Milliseconds 75 }; Write-Output 'CERVTERM_LONG_END'"
Invoke-Capture `
  -Name "long-session" `
  -Program "cmd.exe" `
  -Args (New-PowerShellScriptArgs -Name "long-session" -Command $longSessionCommand) `
  -Markers @("CERVTERM_LONG_START", "long-session-line-20", "CERVTERM_LONG_END") `
  -Timeout "10s"

Write-Host "daily-driver smoke passed: $WorkDir"
