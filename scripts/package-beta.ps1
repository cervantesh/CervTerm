param(
  [string]$Version = "v0.2.0-beta.1",
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

$fontDir = Join-Path $pkgDir "font-sources"
New-Item -ItemType Directory -Force -Path $fontDir | Out-Null
$notoCacheDir = Join-Path $OutDir "font-sources"
$notoCachePath = Join-Path $notoCacheDir "NotoColorEmoji.ttf"
$notoCandidates = @(
  $notoCachePath,
  (Join-Path "font-sources" "NotoColorEmoji.ttf")
)
$notoSource = $notoCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $notoSource) {
  New-Item -ItemType Directory -Force -Path $notoCacheDir | Out-Null
  $notoUrl = "https://github.com/googlefonts/noto-emoji/raw/main/fonts/NotoColorEmoji.ttf"
  Write-Host "Downloading Noto Color Emoji from $notoUrl"
  Invoke-WebRequest -Uri $notoUrl -OutFile $notoCachePath
  $notoSource = $notoCachePath
}
Copy-Item $notoSource -Destination (Join-Path $fontDir "NotoColorEmoji.ttf")
Copy-Item (Join-Path "internal/fontglyph/testdata" "NotoEmoji-LICENSE.txt") -Destination (Join-Path $fontDir "NotoEmoji-LICENSE.txt")

if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
Compress-Archive -Path (Join-Path $pkgDir "*") -DestinationPath $zipPath
Write-Host "Wrote $zipPath"
