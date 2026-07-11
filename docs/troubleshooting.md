# CervTerm Troubleshooting

This guide is for beta users and maintainers diagnosing startup, rendering, config, and release-package issues.

## Collect diagnostics first

Run CervTerm's diagnostic summary first:

```powershell
.\cervterm.exe --doctor
```

Then run CervTerm with an explicit log file if you need a detailed startup log:

```powershell
.\cervterm.exe --log-file .\cervterm.log
```

The log should include startup diagnostics and panic stack traces if the process exits unexpectedly.

For scripted smoke tests, use stderr-only logging:

```powershell
.\cervterm.exe --log-file - --version
```

## Verify the binary and package

From an extracted release zip:

```powershell
.\cervterm.exe --version
.\cervterm.exe --build-info
.\cervterm.exe --print-default-config > cervterm.lua
.\cervterm.exe --doctor
```

The Windows zip should contain:

- `cervterm.exe`
- `cervterm.lua`
- `README.md`
- `CHANGELOG.md`
- `font-sources/NotoColorEmoji.ttf`
- `font-sources/NotoEmoji-LICENSE.txt`
- `docs/`
- `packaging/`

Use the package smoke script from the repo to verify a zip:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/smoke-installed-package.ps1 -ZipPath dist/cervterm-<tag>-windows.zip
```

## Config loading problems

Start from the generated default config:

```powershell
.\cervterm.exe --print-default-config > cervterm.lua
.\cervterm.exe --config .\cervterm.lua --log-file .\cervterm.log
```

If a custom config fails:

1. Re-run `--doctor` and confirm the discovered or overridden config path.
2. Temporarily remove custom shell/font/theme settings.
3. Confirm whether the config is Lua (`.lua`) or Teal (`.tl`).
4. If using Teal, verify the external `tl` command is installed and works.

## Emoji or flag rendering problems

Country flags require `NotoColorEmoji.ttf` for reliable flag glyphs. The release zip should include it under:

```text
font-sources/NotoColorEmoji.ttf
```

If flags render as regional letters, check:

1. The file exists next to the installed executable under `font-sources/`.
2. `--doctor` and the diagnostics log do not contain `emoji coverage warning`.
3. The app was launched from the extracted package directory or a location that preserves `font-sources/`.

Developers can run the glyph checker from the repo:

```bash
go run ./scripts/check-emoji-glyphs.go .tmp/emoji-test-latest.txt
```

Expected healthy output includes:

- all fully-qualified emoji rasterized;
- all visible;
- all color glyphs;
- flags via Noto.

## Unsigned Windows binary warning

Current beta zips are unsigned. Windows SmartScreen or antivirus software may warn because CervTerm does not yet have Authenticode signing reputation.

Before running a downloaded zip:

1. Download from the official GitHub release.
2. Verify `SHA256SUMS.txt`.
3. Review [`docs/release-trust.md`](release-trust.md).

## VT capture for bug reports

When reporting terminal rendering bugs, capture raw PTY output if possible:

```powershell
.\cervterm.exe --capture-vt .\bug.vt --capture-program powershell.exe --capture-timeout 30s --log-file .\capture.log
```

Attach:

- `bug.vt`
- `capture.log`
- `cervterm.lua` if custom config was used
- screenshot or screen recording when the issue is visual
- Windows version and GPU/driver if relevant

## Package smoke checklist

For a clean extracted Windows package:

1. `cervterm.exe --version` prints the release tag.
2. `cervterm.exe --build-info` reports `windows/amd64`.
3. `cervterm.exe --print-default-config` emits Lua config.
4. `--capture-vt` can launch `cmd.exe` and write a non-empty `.vt` file.
5. `font-sources/NotoColorEmoji.ttf` exists.
6. Logs do not contain unexpected emoji font warnings.
