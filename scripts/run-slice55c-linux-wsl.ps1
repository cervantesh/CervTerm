param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("packages", "root")]
    [string]$Suite
)

$ErrorActionPreference = "Stop"
$repo = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$guestScript = @'
set -euo pipefail
repo=$(wslpath -a -u -- "$1")
cd -- "$repo"
if ! command -v go >/dev/null 2>&1; then
    printf 'UNAVAILABLE: Go toolchain not found in Ubuntu-24.04\n' >&2
    exit 3
fi
case "$2" in
    packages)
        CGO_ENABLED=0 go test ./internal/fontglyph/cache ./internal/fontglyph/discovery ./internal/fontglyph/internal/face ./internal/fontglyph/platform ./internal/fontglyph/raster ./internal/fontglyph/shape -count=1
        ;;
    root)
        CGO_ENABLED=0 go test ./internal/fontglyph -run 'Test(L402|NotoColorEmojiSubset|DetectColorTables|Subpixel|OpenTypeBackendRasterizes)' -count=1
        ;;
    *)
        printf 'unknown suite: %s\n' "$2" >&2
        exit 2
        ;;
esac
'@

& wsl.exe -d Ubuntu-24.04 -- bash -lc $guestScript bash $repo $Suite
exit $LASTEXITCODE
