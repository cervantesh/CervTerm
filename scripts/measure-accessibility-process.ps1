param(
  [Parameter(Mandatory = $true)][string]$BaselineExe,
  [Parameter(Mandatory = $true)][string]$BaselineConfig,
  [Parameter(Mandatory = $true)][string]$CandidateExe,
  [Parameter(Mandatory = $true)][string]$DisabledConfig,
  [Parameter(Mandatory = $true)][string]$EnabledConfig,
  [string]$OutFile = "dist/accessibility-process-measurements.csv",
  [string]$RuntimeMetricsDir = "",
  [ValidateRange(3, 20)][int]$Iterations = 3,
  [ValidateRange(1, 30)][int]$IdleSeconds = 3,
	[ValidateRange(1, 10)][int]$WarmupSeconds = 1
)

$ErrorActionPreference = "Stop"

function Resolve-InputPath([string]$Path) {
  return (Resolve-Path -LiteralPath $Path).Path
}

function Measure-CervTerm([string]$Name, [string]$Exe, [string]$Config) {
  $exeHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Exe).Hash
	$configHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Config).Hash
  1..$Iterations | ForEach-Object {
    $run = $_
	$priorMetricsOut = $env:CERVTERM_RUNTIME_METRICS_OUT
	$priorMetricsDelay = $env:CERVTERM_RUNTIME_METRICS_DELAY
	$priorMetricsWarmup = $env:CERVTERM_RUNTIME_METRICS_WARMUP
	$metricsPath = $null
	if ($RuntimeMetricsDir) {
	  New-Item -ItemType Directory -Force -Path $RuntimeMetricsDir | Out-Null
	  $metricsPath = Join-Path $RuntimeMetricsDir "$Name-$run.json"
	  $env:CERVTERM_RUNTIME_METRICS_OUT = $metricsPath
	  $env:CERVTERM_RUNTIME_METRICS_DELAY = "${IdleSeconds}s"
	  $env:CERVTERM_RUNTIME_METRICS_WARMUP = "${WarmupSeconds}s"
	}
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
	  Start-Sleep -Seconds $WarmupSeconds
      $cpuBefore = $process.TotalProcessorTime.TotalMilliseconds
      Start-Sleep -Seconds $IdleSeconds
      $process.Refresh()
      $runtimeMetrics = $null
	  if ($metricsPath) {
	    if (-not (Test-Path -LiteralPath $metricsPath)) { throw "$Name runtime metrics were not written" }
	    $runtimeMetrics = Get-Content -Raw -LiteralPath $metricsPath | ConvertFrom-Json
	  }
      [pscustomobject]@{
        Name = $Name
		ExePath = $Exe
		ExeSHA256 = $exeHash
		ConfigPath = $Config
		ConfigSHA256 = $configHash
        Run = $run
        ReadyMS = [math]::Round($readyMS, 2)
        WorkingSetMiB = [math]::Round($process.WorkingSet64 / 1MB, 2)
        PrivateMiB = [math]::Round($process.PrivateMemorySize64 / 1MB, 2)
        IdleCPUms = [math]::Round($process.TotalProcessorTime.TotalMilliseconds - $cpuBefore, 2)
        IdleSeconds = $IdleSeconds
		WarmupSeconds = $WarmupSeconds
        Handles = $process.HandleCount
        HeapMiB = $(if ($runtimeMetrics) { [math]::Round($runtimeMetrics.heap_alloc / 1MB, 2) } else { $null })
		Wakes = $(if ($runtimeMetrics) { $runtimeMetrics.wakes } else { $null })
		Frames = $(if ($runtimeMetrics) { $runtimeMetrics.frames } else { $null })
		GoAllocs = $(if ($runtimeMetrics) { $runtimeMetrics.allocs } else { $null })
      }
    } finally {
      if (-not $process.HasExited) { $process.Kill() }
      $process.WaitForExit()
      $process.Dispose()
      $env:CERVTERM_RUNTIME_METRICS_OUT = $priorMetricsOut
	  $env:CERVTERM_RUNTIME_METRICS_DELAY = $priorMetricsDelay
	  $env:CERVTERM_RUNTIME_METRICS_WARMUP = $priorMetricsWarmup
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
