param(
  [Parameter(Mandatory = $true)][string]$BaselineExe,
  [Parameter(Mandatory = $true)][string]$BaselineConfig,
  [Parameter(Mandatory = $true)][string]$CandidateExe,
  [Parameter(Mandatory = $true)][string]$DisabledConfig,
  [Parameter(Mandatory = $true)][string]$EnabledConfig,
  [string]$OutFile = "dist/accessibility-process-measurements.csv",
  [ValidateRange(3, 20)][int]$Iterations = 3,
  [ValidateRange(1, 30)][int]$IdleSeconds = 3
)

$ErrorActionPreference = "Stop"

function Resolve-InputPath([string]$Path) {
  return (Resolve-Path -LiteralPath $Path).Path
}

function Measure-CervTerm([string]$Name, [string]$Exe, [string]$Config) {
  1..$Iterations | ForEach-Object {
    $timer = [Diagnostics.Stopwatch]::StartNew()
    $process = Start-Process -FilePath $Exe -ArgumentList @("--config", $Config, "--log-file", "-") -PassThru
    try {
      $ready = $false
      while ($timer.ElapsedMilliseconds -lt 10000) {
        Start-Sleep -Milliseconds 25
        $process.Refresh()
        if ($process.HasExited) { throw "$Name exited before exposing a responding window" }
        if ($process.MainWindowHandle -ne 0 -and $process.Responding) {
          $ready = $true
          break
        }
      }
      if (-not $ready) { throw "$Name did not expose a responding window within 10 seconds" }
      $readyMS = $timer.Elapsed.TotalMilliseconds
      $cpuBefore = $process.TotalProcessorTime.TotalMilliseconds
      Start-Sleep -Seconds $IdleSeconds
      $process.Refresh()
      [pscustomobject]@{
        Name = $Name
        Run = $_
        ReadyMS = [math]::Round($readyMS, 2)
        WorkingSetMiB = [math]::Round($process.WorkingSet64 / 1MB, 2)
        PrivateMiB = [math]::Round($process.PrivateMemorySize64 / 1MB, 2)
        IdleCPUms = [math]::Round($process.TotalProcessorTime.TotalMilliseconds - $cpuBefore, 2)
        IdleSeconds = $IdleSeconds
        Handles = $process.HandleCount
      }
    } finally {
      if (-not $process.HasExited) { $process.Kill() }
      $process.WaitForExit()
      $process.Dispose()
    }
  }
}

$baselineExePath = Resolve-InputPath $BaselineExe
$baselineConfigPath = Resolve-InputPath $BaselineConfig
$candidateExePath = Resolve-InputPath $CandidateExe
$disabledConfigPath = Resolve-InputPath $DisabledConfig
$enabledConfigPath = Resolve-InputPath $EnabledConfig
$results = @(
  Measure-CervTerm "baseline" $baselineExePath $baselineConfigPath
  Measure-CervTerm "candidate-disabled" $candidateExePath $disabledConfigPath
  Measure-CervTerm "candidate-enabled" $candidateExePath $enabledConfigPath
)
$parent = Split-Path -Parent $OutFile
if ($parent) { New-Item -ItemType Directory -Force -Path $parent | Out-Null }
$results | Export-Csv -NoTypeInformation -Path $OutFile
$results | Format-Table -AutoSize
