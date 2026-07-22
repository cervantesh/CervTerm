# CervTerm Troubleshooting

This guide is for beta users and maintainers diagnosing startup, rendering, config, and release-package issues.

## Collect diagnostics first

Run CervTerm's diagnostic summary first:

```cmd
.\cervterm.exe --doctor
```

Then run CervTerm with an explicit log file if you need a detailed startup log:

```cmd
.\cervterm.exe --log-file .\cervterm.log
```

The log should include startup diagnostics and panic stack traces if the process exits unexpectedly.

For scripted smoke tests, use stderr-only logging:

```cmd
.\cervterm.exe --log-file - --version
```

## Verify the binary and package

From an extracted release zip:

```cmd
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

```cmd
go run ./scripts/smoke-installed-package.go -zip dist/cervterm-<tag>-windows.zip
```

## Config loading problems

Start from the generated default config:

```cmd
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

```cmd
.\cervterm.exe --capture-vt .\bug.vt --capture-program cmd.exe --capture-timeout 30s --log-file .\capture.log
```

Attach:

- `bug.vt`
- `capture.log`
- `cervterm.lua` if custom config was used
- screenshot or screen recording when the issue is visual
- Windows version and GPU/driver if relevant

## Experimental Kitty graphics diagnostics

Kitty graphics are experimental, disabled by default, restart-scoped, and limited to the GLFW/OpenGL frontend. If a direct-data image does not appear:

1. Confirm the active v2 config contains `graphics = { kitty = { enabled = true } }`, then fully restart CervTerm; live reload does not activate this feature.
2. Confirm the application uses only actions `t`, `T`, `p`, `d`, or `q` and direct data (`t=d` or the default direct transport).
3. Confirm the format is RGB24 (`f=24`), RGBA32 (`f=32`), or PNG (`f=100`). `o=z` zlib is accepted only with raw RGB24/RGBA32, not PNG.
4. Check for a fixed reply code and for cap/timeout diagnostics. Oversized, malformed, unsupported, cancelled, expired, or incomplete transfers commit no partial image or placement.
5. Reproduce with the smallest image and attach `--doctor` output, the diagnostics log, GPU/driver details, and the emitting application's exact Kitty control sequence with any sensitive payload removed.

Replies have the form `ESC_Ga=<action>;<code>ESC\` and never echo payload data or paths. Codes are:

- `OK`: accepted and committed;
- `EINVAL`: malformed or invalid request;
- `ENOTSUP`: unsupported action, key, format, compression, or transport;
- `ENOSPC`: a chunk, transfer, image, placement, reply, decode, or GPU cap was reached;
- `ETIME`: transfer lifetime or acceptance deadline expired;
- `ECANCELED`: transfer was cancelled;
- `ENOENT`: referenced image or placement was not found;
- `EIO`: bounded internal/runtime failure.

Kitty `q=1` suppresses successful replies; `q=2` suppresses all replies. Lack of a reply under those modes is not evidence that a request succeeded. `--doctor` reports configured intent and limits from a separate one-shot process; it does not prove that the live OpenGL frontend activated the feature.

For immediate rollback, set `graphics.kitty.enabled = false` and restart CervTerm. CervTerm does not claim full Kitty conformance and does not support animation, external file/path/temporary-file/shared-memory transports, Unicode placeholders, Sixel, iTerm inline images, or non-OpenGL image rendering.

## Package smoke checklist

For a clean extracted Windows package:

1. `cervterm.exe --version` prints the release tag.
2. `cervterm.exe --build-info` reports `windows/amd64`.
3. `cervterm.exe --print-default-config` emits Lua config.
4. `--capture-vt` can launch `cmd.exe` and write a non-empty `.vt` file.
5. `font-sources/NotoColorEmoji.ttf` exists.
6. Logs do not contain unexpected emoji font warnings.
