param(
  [Parameter(Mandatory = $true)]
  [string]$ZipPath,
  [string]$WorkDir = "dist/installed-package-smoke",
  [string]$ExpectedVersion = ""
)

$ErrorActionPreference = "Stop"

$zip = Resolve-Path $ZipPath
if (Test-Path $WorkDir) { Remove-Item -Recurse -Force $WorkDir }
New-Item -ItemType Directory -Force -Path $WorkDir | Out-Null
Expand-Archive -Force -Path $zip -DestinationPath $WorkDir

$exe = Join-Path $WorkDir "cervterm.exe"
if (-not (Test-Path $exe)) { throw "package missing cervterm.exe" }

$noto = Join-Path $WorkDir "font-sources/NotoColorEmoji.ttf"
$notoLicense = Join-Path $WorkDir "font-sources/NotoEmoji-LICENSE.txt"
if (-not (Test-Path $noto)) { throw "package missing font-sources/NotoColorEmoji.ttf" }
if (-not (Test-Path $notoLicense)) { throw "package missing font-sources/NotoEmoji-LICENSE.txt" }

$version = (& $exe --version).Trim()
if ($ExpectedVersion -and $version -ne $ExpectedVersion) {
  throw "version mismatch: got '$version', want '$ExpectedVersion'"
}
Write-Host "version: $version"

$buildInfo = (& $exe --build-info).Trim()
if ($buildInfo -notmatch "CervTerm") { throw "unexpected build info: $buildInfo" }
Write-Host "build-info: $buildInfo"

$doctor = (& $exe --doctor) -join "`n"
if ($doctor -notmatch "CervTerm doctor") { throw "doctor output missing header" }
if ($doctor -notmatch "diagnostics:") { throw "doctor output missing diagnostics section" }
if ($doctor -notmatch "config:") { throw "doctor output missing config section" }
Write-Host "doctor: ok"

$config = & $exe --print-default-config
if (($config -join "`n") -notmatch "return \{") { throw "default config did not look like Lua" }
$configPath = Join-Path $WorkDir "generated-default.lua"
$config | Set-Content -Encoding UTF8 $configPath

$vtPath = Join-Path $WorkDir "smoke.vt"
$logPath = Join-Path $WorkDir "smoke.log"
& $exe --capture-vt $vtPath --capture-program cmd.exe --capture-arg /C --capture-arg "echo cervterm-smoke" --capture-timeout 10s --log-file $logPath

if (-not (Test-Path $vtPath)) { throw "capture-vt did not create $vtPath" }
if ((Get-Item $vtPath).Length -le 0) { throw "capture-vt output is empty" }
if (-not (Test-Path $logPath)) { throw "smoke log was not created" }

$logText = Get-Content -Raw $logPath
if ($logText -match "emoji coverage warning") {
  throw "unexpected emoji coverage warning in installed package smoke log"
}

Write-Host "installed package smoke passed: $ZipPath"
