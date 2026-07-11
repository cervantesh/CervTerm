param(
  [string]$Version = "0.1.0-beta.1",
  [string]$OutDir = "dist"
)

$ErrorActionPreference = "Stop"
$pkgDir = Join-Path $OutDir "cervterm-$Version-windows"
$zipPath = Join-Path $OutDir "cervterm-$Version-windows.zip"

if (Test-Path $pkgDir) { Remove-Item -Recurse -Force $pkgDir }
New-Item -ItemType Directory -Force -Path $pkgDir | Out-Null

$exe = Join-Path $pkgDir "cervterm.exe"
go build -tags glfw -ldflags "-X cervterm/internal/buildinfo.Version=$Version" -o $exe ./cmd/cervterm
& $exe --print-default-config | Set-Content -Encoding UTF8 (Join-Path $pkgDir "cervterm.lua")
Copy-Item README.md, CHANGELOG.md -Destination $pkgDir
Copy-Item docs -Destination (Join-Path $pkgDir "docs") -Recurse
Copy-Item packaging -Destination (Join-Path $pkgDir "packaging") -Recurse

if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
Compress-Archive -Path (Join-Path $pkgDir "*") -DestinationPath $zipPath
Write-Host "Wrote $zipPath"
