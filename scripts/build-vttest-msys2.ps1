param(
  [string]$Msys2Bash = "C:\msys64\usr\bin\bash.exe",
  [string]$OutDir = "dist\tools\vttest-msys2",
  [string]$Url = "https://invisible-island.net/datafiles/release/vttest.tar.gz"
)

$ErrorActionPreference = "Stop"
if (-not (Test-Path $Msys2Bash)) {
  throw "MSYS2 bash not found at $Msys2Bash. Install MSYS2, then run: pacman -S --needed base-devel gcc make ncurses-devel"
}

$root = (Resolve-Path ".").Path
$out = Join-Path $root $OutDir
New-Item -ItemType Directory -Force -Path $out | Out-Null
$msysOut = $out -replace '\\', '/'
if ($msysOut -match '^([A-Za-z]):/(.*)$') {
  $drive = $matches[1].ToLower()
  $rest = $matches[2]
  $msysOut = "/$drive/$rest"
}

$script = @"
set -euo pipefail
mkdir -p '$msysOut'
cd '$msysOut'
if [ ! -d src ]; then
  curl -L --fail -o vttest.tar.gz '$Url'
  mkdir src
  tar -xzf vttest.tar.gz -C src --strip-components=1
fi
cd src
./configure --prefix='$msysOut/install'
make -j2
make install
if [ -x '$msysOut/install/bin/vttest.exe' ]; then
  echo '$msysOut/install/bin/vttest.exe'
elif [ -x '$msysOut/src/vttest.exe' ]; then
  echo '$msysOut/src/vttest.exe'
else
  echo 'vttest build finished but executable was not found' >&2
  exit 1
fi
"@

& $Msys2Bash -lc $script
