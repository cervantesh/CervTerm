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

Kitty, Sixel, and iTerm inline images are experimental, disabled by default, restart-scoped, rendered only by the GLFW/OpenGL frontend, and not platform-qualified. If an image does not appear:

1. Confirm an explicit v2 config enables the intended `graphics.kitty`, `graphics.sixel`, or `graphics.iterm` flag, then fully restart; live reload records a restart-required difference but cannot activate graphics.
2. Start with only one protocol enabled and built-in limits. `--doctor` reports configured intent/limits from a separate process, not live OpenGL activation.
3. For Kitty, use only direct-data `t`/`T`/`p`/`d`/`q`, RGB24/RGBA32/PNG, and raw-only zlib.
4. For Sixel, use only 7-bit `ESC P q|0q|0;0q|0;0;0q ... ST`, one `"1;1;W;H` declaration before pixels, and the documented raster/RGB/repeat/`$`/`-` tokens. BEL does not terminate Sixel.
5. For iTerm, use only 7-bit `OSC 1337;File=...:<strict padded base64>` ending in BEL or ST, with exact lexical `inline=1`, positive decoded `size`, one exact-EOF PNG, optional cell-only `width` xor `height` in `1..256`, and absent or exact `preserveAspectRatio=1`.
6. Reproduce with the smallest fixture and attach redacted emitting commands, config, `--doctor`, log, Windows build, GPU/driver, and OpenGL vendor/renderer/version. Do not attach private image payloads.

Kitty replies have the form `ESC_Ga=<action>;<code>ESC\` and never echo payload or paths. Codes are `OK`, `EINVAL`, `ENOTSUP`, `ENOSPC`, `ETIME`, `ECANCELED`, `ENOENT`, and `EIO`; `q=1` suppresses success and `q=2` suppresses all replies. Sixel and iTerm deliberately emit **no reply** and reserve no reply slot, so silence is not proof of success.

Phase 14 programmatic diagnostics are limited to protocol (`sixel` or `iterm`), fixed reason (`invalid`, `unsupported`, `limit`, `timeout`, `cancelled`, `failed`, `stale`, or `busy`), count, and duration. They must never contain payload, pixels, metadata names, base64, pane/image/transfer/placement IDs, paths, or URLs. Treat any such leakage as a security bug.

The mux `PaneOutput`/Lua output stream is intentionally not raw PTY ingress when an image adapter is enabled. Parser-coupled projection removes an enabled selected Kitty/Sixel/iTerm control-string envelope across fragmentation and EOF while preserving disabled or unselected control strings. If a selected payload marker appears in a Lua output callback, stop testing and report it with the payload itself removed. Selected-only ingress may legitimately produce an empty `PaneOutput` event and no Lua output callback while still dirtying the pane.

Unknown/duplicate fields, malformed base64/PNG, HLS, C1 forms, broad iTerm sizing, animation, Sixel scrolling/DECSDM, cursor effects, and file/path/URL/temporary-file/shared-memory/download/write forms reject atomically. The bounded Sixel/iTerm subsets perform no external filesystem, network, process, or unsafe I/O; do not work around rejection by granting extra file/network access.

For immediate independent rollback, set only the affected `graphics.<protocol>.enabled = false` and restart. If startup, child-window, or restore activation fails, CervTerm should retain old-or-new state and close provisional resources in reverse order; failed or late decode must not replace prior state. Full/broad Kitty, Sixel, and iTerm conformance and non-OpenGL image rendering remain outside the claim. Every Phase 14 real-GUI row is currently UNRUN; see [`manual-verification.md`](manual-verification.md#phase-14-experimental-sixel-and-iterm-images--real-gui-qualification).

## Package smoke checklist

For a clean extracted Windows package:

1. `cervterm.exe --version` prints the release tag.
2. `cervterm.exe --build-info` reports `windows/amd64`.
3. `cervterm.exe --print-default-config` emits Lua config.
4. `--capture-vt` can launch `cmd.exe` and write a non-empty `.vt` file.
5. `font-sources/NotoColorEmoji.ttf` exists.
6. Logs do not contain unexpected emoji font warnings.
