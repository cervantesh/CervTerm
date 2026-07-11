param(
  [string]$Version = "v0.2.0-beta.1",
  [string]$OutDir = "dist",
  [string]$WindowsZip = "",
  [string]$VttestExe = "",
  [switch]$RequireSigning,
  [switch]$RequireVttest,
  [switch]$RequireWix
)

$ErrorActionPreference = "Stop"

$checks = New-Object System.Collections.Generic.List[object]
function Add-Check([string]$Name, [bool]$Passed, [string]$Detail, [bool]$Required = $true) {
  $checks.Add([pscustomobject]@{ Name = $Name; Passed = $Passed; Required = $Required; Detail = $Detail }) | Out-Null
}
function Find-CommandPath([string]$Name) {
  $cmd = Get-Command $Name -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($cmd) { return $cmd.Source }
  return $null
}

$root = (Resolve-Path ".").Path
$resolvedOut = Join-Path $root $OutDir
if (-not $WindowsZip) {
  $WindowsZip = Join-Path $resolvedOut "cervterm-$Version-windows.zip"
}

Add-Check "go toolchain" ([bool](Find-CommandPath "go")) "go must be on PATH for builds and tests"
Add-Check "git checkout" (Test-Path (Join-Path $root ".git")) "run preflight from the CervTerm repository root"
Add-Check "release workflow" (Test-Path (Join-Path $root ".github/workflows/release.yml")) "tagged release workflow should exist"
Add-Check "package script" (Test-Path (Join-Path $root "scripts/package-beta.ps1")) "local beta package script should exist"
Add-Check "signing script" (Test-Path (Join-Path $root "scripts/sign-windows-exe.ps1")) "Authenticode signing hook should exist"
Add-Check "vttest build route" (Test-Path (Join-Path $root "scripts/build-vttest-msys2.ps1")) "MSYS2 vttest build helper should exist"
Add-Check "vttest capture route" (Test-Path (Join-Path $root "scripts/capture-vttest.ps1")) "raw vttest capture helper should exist"
Add-Check "WiX template" (Test-Path (Join-Path $root "packaging/wix/CervTerm.wxs")) "MSI template should exist"
Add-Check "winget templates" (Test-Path (Join-Path $root "packaging/winget/T50Systems.CervTerm.yaml")) "portable winget templates should exist"
Add-Check "installed package smoke script" (Test-Path (Join-Path $root "scripts/smoke-installed-package.ps1")) "clean package smoke should be reusable locally and in CI"
Add-Check "maturity gates script" (Test-Path (Join-Path $root "scripts/check-maturity-gates.go")) "maturity guardrails should be executable in CI"
Add-Check "troubleshooting docs" (Test-Path (Join-Path $root "docs/troubleshooting.md")) "user diagnostics workflow should be documented"
Add-Check "release trust docs" (Test-Path (Join-Path $root "docs/release-trust.md")) "checksums, attestations, and unsigned beta status should be documented"
Add-Check "maturity improvement plan" (Test-Path (Join-Path $root "docs/maturity-improvement-plan.md")) "DoE/DoDm maturity improvement plan should exist"

$hasPfx = -not [string]::IsNullOrWhiteSpace($env:WINDOWS_CODESIGN_PFX_BASE64)
$hasPfxPassword = -not [string]::IsNullOrWhiteSpace($env:WINDOWS_CODESIGN_PASSWORD)
$signtool = Find-CommandPath "signtool.exe"
if (-not $signtool) {
  $kits = Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"
  if (Test-Path $kits) {
    $signtool = Get-ChildItem $kits -Filter signtool.exe -Recurse -ErrorAction SilentlyContinue |
      Where-Object { $_.FullName -match '\\x64\\signtool\.exe$' } |
      Sort-Object FullName -Descending |
      Select-Object -First 1 -ExpandProperty FullName
  }
}
if ($RequireSigning.IsPresent) {
  Add-Check "Authenticode secrets" ($hasPfx -and $hasPfxPassword) "set WINDOWS_CODESIGN_PFX_BASE64 and WINDOWS_CODESIGN_PASSWORD" $true
  Add-Check "signtool" ([bool]$signtool) "install Windows SDK or run release on windows-latest" $true
} else {
  Add-Check "Authenticode signing" $true "intentionally deferred for free beta zip releases; SHA256SUMS and GitHub attestations remain the default" $false
}

$vttestAvailable = $false
if ($VttestExe) {
  $vttestAvailable = Test-Path $VttestExe
} else {
  $vttestAvailable = [bool](Find-CommandPath "vttest.exe") -or [bool](Find-CommandPath "vttest") -or (Test-Path (Join-Path $root "dist/tools/vttest-msys2/install/bin/vttest.exe"))
}
$msys2Bash = Test-Path "C:\msys64\usr\bin\bash.exe"
Add-Check "vttest executable" $vttestAvailable "provide -VttestExe, install vttest, or run scripts/build-vttest-msys2.ps1" $RequireVttest.IsPresent
Add-Check "MSYS2 bash" $msys2Bash "install MSYS2 if building vttest locally on Windows" $false

if ($RequireWix.IsPresent) {
  $wix = Find-CommandPath "wix.exe"
  Add-Check "WiX CLI" ([bool]$wix) "install WiXToolset.WiXToolset before publishing MSI artifacts" $true
  Add-Check "MSI policy" $false "decide per-user/per-machine, PATH/shortcut behavior, upgrade cadence, config install behavior, and signing policy before enabling MSI CI" $true
} else {
  Add-Check "MSI/WiX publishing" $true "intentionally deferred; portable zip and winget templates are the beta distribution path" $false
}

if (Test-Path $WindowsZip) {
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  $archive = [System.IO.Compression.ZipFile]::OpenRead((Resolve-Path $WindowsZip))
  try {
    $entries = $archive.Entries.FullName | ForEach-Object { $_ -replace "\\", "/" }
    foreach ($required in @("cervterm.exe", "cervterm.lua", "README.md", "CHANGELOG.md", "font-sources/NotoColorEmoji.ttf", "font-sources/NotoEmoji-LICENSE.txt", "docs/product-roadmap.md", "docs/troubleshooting.md", "docs/release-trust.md", "docs/maturity-improvement-plan.md", "docs/assets/cervterm-preview.png", "packaging/winget/README.md")) {
      Add-Check "zip contains $required" ($entries -contains $required) "check $WindowsZip package contents"
    }
  } finally {
    $archive.Dispose()
  }
} else {
  Add-Check "Windows beta zip" $false "run scripts/package-beta.ps1 -Version $Version -OutDir $OutDir" $false
}

$failedRequired = @($checks | Where-Object { $_.Required -and -not $_.Passed })
$failedOptional = @($checks | Where-Object { -not $_.Required -and -not $_.Passed })

foreach ($check in $checks) {
  $status = if ($check.Passed) { "PASS" } elseif ($check.Required) { "FAIL" } else { "WARN" }
  Write-Host ("[{0}] {1}: {2}" -f $status, $check.Name, $check.Detail)
}

Write-Host ""
Write-Host ("Release preflight summary: {0} pass, {1} required fail, {2} warning" -f @($checks | Where-Object Passed).Count, $failedRequired.Count, $failedOptional.Count)

if ($failedRequired.Count -gt 0) {
  exit 1
}
