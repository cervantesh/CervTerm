param(
  [Parameter(Mandatory=$true)][string]$SourceFont,
  [Parameter(Mandatory=$true)][string]$OutputFont,
  [string]$Text = "CervTerm 😀 👩‍💻 é अ م",
  [string]$Tool = "pyftsubset"
)

$ErrorActionPreference = "Stop"
if (-not (Test-Path $SourceFont)) {
  throw "Source font not found: $SourceFont"
}
if (-not (Get-Command $Tool -ErrorAction SilentlyContinue)) {
  throw "pyftsubset not found. Install fonttools in a Python environment first."
}

$licenseHint = @("OFL", "Apache", "MIT")
Write-Host "Only commit generated subsets when the source font license permits redistribution (for example: $($licenseHint -join ', '))."
& $Tool $SourceFont --output-file=$OutputFont --text=$Text --layout-features='*' --glyph-names --symbol-cmap --legacy-cmap --notdef-glyph --notdef-outline --recommended-glyphs --name-IDs='*' --name-legacy --name-languages='*'
Write-Host "Wrote $OutputFont"
