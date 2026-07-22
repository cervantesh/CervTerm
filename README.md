# CervTerm

CervTerm is an experimental GPU terminal emulator written in Go.

![CervTerm running a local Windows shell](docs/assets/cervterm-preview.png)

## Current status

CervTerm is not a finished daily-driver terminal yet, but it already includes:

- Windows ConPTY backend and Unix PTY backend behind build tags.
- GLFW/OpenGL frontend.
- Experimental, default-off, restart-scoped direct-data Kitty graphics subset on the GLFW/OpenGL frontend (`t`/`T`/`p`/`d`/`q`; RGB24, RGBA32, and PNG; zlib only for raw RGB/RGBA), with bounded chunking and resource caps.
- Scrollback, alternate screen, resize reflow, selection/copy/paste, and bracketed paste.
- Native in-process pane mux with independent PTY/parser/core state per pane, clipped binary row/column splits, draggable dividers, focused input, deterministic close/collapse, and independent per-pane zoom.
- Scrollback search: `ctrl+shift+f` opens a search bar (Enter jumps to the next match upward, Esc closes); also scriptable via `term:search`.
- VT parsing for common cursor, erase, color, scroll-region, insert/delete, input-mode, mouse-mode, and title sequences.
- Keyboard encoding for navigation keys, F1-F12, and Ctrl/Alt/Shift modifiers.
- SGR mouse press/release/wheel/drag encoding, including modifiers.
- Lua config loading, Teal check/gen support, atomic runtime reload, and `--print-default-config`.
- Per-side padding; independent text/background opacity; bounded solid, gradient, and image background layers; configurable scrollbar visibility/stable gutter/fade FPS; and capability-aware native blur.
- Renderer-neutral OpenType glyph backend with bitmap color fonts, broad COLRv1 paint/composite/variation support, SVG glyph extraction/rasterization, DirectWrite shaping smoke coverage, and shaped color cluster handling.
- Diagnostics logging via `--log-file` / `CERVTERM_LOG_FILE`, including panic stack capture.
- Parser fuzz smoke coverage, replay-style VT golden fixtures, and a Windows daily-driver smoke matrix for cmd.exe, git, pager, alternate-screen, resize/reflow, and longer-session paths.

- `--doctor` support diagnostics for config/log/environment reporting.
See:

- [`docs/wezterm-parity-roadmap.md`](docs/wezterm-parity-roadmap.md)
- [`docs/parity-baseline.md`](docs/parity-baseline.md)
- [`docs/parity-support-matrix.json`](docs/parity-support-matrix.json)
- [`docs/product-roadmap.md`](docs/product-roadmap.md)
- [`docs/config-roadmap.md`](docs/config-roadmap.md)
- [`docs/config-compatibility-policy.md`](docs/config-compatibility-policy.md)
- [`docs/vttest-checklist.md`](docs/vttest-checklist.md)
- [`docs/vttest-captures.md`](docs/vttest-captures.md)
- [`docs/emoji-rendering-research.md`](docs/emoji-rendering-research.md)
- [`docs/shaping-options.md`](docs/shaping-options.md)
- [`docs/release-packaging.md`](docs/release-packaging.md)
- [`docs/getting-started.md`](docs/getting-started.md)
- [`docs/daily-driver-smoke.md`](docs/daily-driver-smoke.md)
- [`docs/troubleshooting.md`](docs/troubleshooting.md)
- [`docs/release-trust.md`](docs/release-trust.md)
- [`SUPPORT.md`](SUPPORT.md)

## Install beta builds

Tagged releases publish portable zip artifacts from GitHub Actions:

- `cervterm-<tag>-windows.zip` contains the GLFW Windows executable, generated default config, bundled `font-sources/NotoColorEmoji.ttf`, README, CHANGELOG, docs, and packaging metadata.
- `cervterm-<tag>-linux-headless-amd64.zip` contains the headless command for Unix PTY/config/capture smoke coverage before a Linux GUI frontend is packaged.

For a Windows zip install:

1. Download and extract the Windows zip from the GitHub release.
2. Run `cervterm.exe --version`, `cervterm.exe --build-info`, or `cervterm.exe --doctor` to verify the binary and environment.
3. Generate a starter config with `cervterm.exe --print-default-config > cervterm.lua`.
4. Launch with `cervterm.exe --config cervterm.lua`.
5. If diagnosing startup issues, add `--log-file cervterm.log`.

Portable winget manifest templates live under `packaging/winget/`. Authenticode signing and MSI/WiX publishing are intentionally deferred for now; beta distribution uses unsigned portable zips with SHA256 checksums and GitHub provenance attestations.

For release authenticity and unsigned beta expectations, see [`docs/release-trust.md`](docs/release-trust.md). For diagnostics, see [`docs/troubleshooting.md`](docs/troubleshooting.md).

## Pane shortcuts

Lua keybindings take precedence over these built-ins:

- `Alt+Shift+=`: split the focused pane into left/right columns.
- `Alt+Shift+-`: split the focused pane into top/bottom rows.
- `Alt+Left/Right/Up/Down`: move focus geometrically.
- `Ctrl+Shift+W`: close the focused pane (or the window when it is the final pane).
- Drag a divider with the left mouse button to resize adjacent panes.
- `Ctrl++`, `Ctrl+-`, `Ctrl+0`, and Ctrl+wheel zoom only the focused pane; sibling panes keep their font size and grid.

The mux is local and in-process. Pane zoom shares one fixed-size glyph atlas across all active font sizes, so focus changes do not rebuild GPU resources. Visible tabs, detachable/persistent sessions, remote domains and tmux integration are deferred.


## Build and test

```sh
go test ./...
go test -tags glfw ./internal/fontglyph ./internal/frontend/glfwgl ./cmd/cervterm -count=1
go run ./scripts/capture-parity-baseline.go -count 3
go build -tags glfw -o cervterm.exe ./cmd/cervterm
```

Run the release/package preflight after creating a local beta zip:

```cmd
go run ./scripts/package-beta.go -version <tag> -outdir dist
go run ./scripts/release-preflight.go -version <tag> -outdir dist -windows-zip dist/cervterm-<tag>-windows.zip
```

Run the Windows daily-driver smoke matrix:

```sh
go run ./scripts/daily-driver-smoke.go -workdir dist/daily-driver-smoke -version daily-smoke
```


Cross-compile smoke for the non-GLFW headless command:

```sh
GOOS=linux GOARCH=amd64 go build -o dist/cervterm-linux-amd64 ./cmd/cervterm
```

## Run

```sh
./cervterm.exe
./cervterm.exe --version
./cervterm.exe --build-info
./cervterm.exe --doctor
./cervterm.exe --print-default-config > cervterm.lua
./cervterm.exe --config path/to/cervterm.lua
./cervterm.exe --capture-vt internal/vt/testdata/manual.vt --capture-program vttest --capture-rows 24 --capture-cols 80
./cervterm.exe --log-file ./cervterm.log
```

## Lua config example

Generate a complete editable template with `--print-default-config`. The default keeps compatibility behavior while exposing Phase 5 appearance/window controls. A minimal v2 override:

```lua
return {
  config_version = 2,
  window = { initial_rows = 30, initial_cols = 100, decorations = "system", titlebar = "dark", opacity = 1.0, text_opacity = 1.0, background_opacity = 1.0, padding_left = 8, padding_right = 8, padding_top = 8, padding_bottom = 8 },
  colors = { background = "#080B12E6" },
  scrolling = { history = 2000, wheel_multiplier = 3, hide_cursor_when_scrolled = true },
  scrollbar = { mode = "scrolling", stable_gutter = true, animation_fps = 30 },
  tab_bar = { mode = "multiple", position = "top", min_width_px = 96, max_width_px = 220 },
  render = { max_fps = 0 },
  shell = { program = "cmd.exe", args = {} },
}
```

`render.max_fps = 0` means uncapped apart from vsync/event policy; a positive value caps presentation without changing the damage-driven idle policy. Renderer selection is intentionally unavailable: GLFW/OpenGL remains the only supported frontend/backend.

The selected source is watched with debounce. `ctrl+shift+r` and `term:reload_config()` request a manual atomic reload; invalid edits preserve the last valid runtime. Shell and other startup-only changes are reported as requiring restart.

## Teal config

`cervterm.tl` is checked and generated through the external `tl` command before CervTerm loads the generated Lua file. Copy `docs/examples/cervterm.d.tl` beside it for the complete root `cervterm.Config` type, or start from `docs/examples/cervterm.tl`. Lua remains the runtime target, so Teal is optional for users who prefer direct Lua config.

## Diagnostics logging

Runtime diagnostics are written to stderr and to a local log file by default. Override the location with `--log-file path/to/cervterm.log` or `CERVTERM_LOG_FILE`; use `--log-file -` to keep diagnostics on stderr only. Run `--doctor` to print the effective log path, config discovery state, environment hints, and support checklist. Unexpected panics are captured with a stack trace before CervTerm exits.

## Display scaling and Phase 5 appearance controls

Per-side logical padding is scaled with DPI and participates in grid sizing and pointer hit testing. The scrollbar supports `always`, `hover`, `scrolling`, and `never`; `stable_gutter` reserves its slot so visibility changes do not resize the PTY, and `animation_fps` bounds fade updates.

	The retained tab bar supports `multiple` (default), `always`, and `hidden` visibility plus top/bottom placement. Its bounded tab widths, add/close controls, active-visible overflow, and geometry reservation reload atomically; the default one-tab window remains pixel-compatible with no bar.

Text opacity and composed background opacity are independent, while `window.opacity` remains whole-window opacity. Validation prevents incompatible simultaneous translucency modes. Background composition is a bounded ordered layer stack: solid colors, linear gradients, and locally decoded images with explicit fit/alignment, decode limits, cache budgets, and asynchronous replacement on resize/reload.

Rendering remains damage-driven by default (`render.redraw = "on_demand"`). `render.max_fps` caps presentation when positive; it does not create frames while idle. `render.vsync` still limits swaps to the monitor refresh. There is no renderer selector.

Initial terminal geometry can be set with `window.rows`/`window.cols`. `window.decorations` and `window.titlebar` request the supported native startup mode and are recreation-scoped; unsupported platform combinations degrade through capability diagnostics rather than implying cross-platform parity. Phase 5 GUI qualification is Windows-only unless a platform pass is explicitly recorded in [`docs/manual-verification.md`](docs/manual-verification.md).

## Phase 6–7 input and retained UX

CervTerm supports bounded leader chords, named key tables, exact typed mouse bindings, and transactional pane resize/swap/move actions. Phase 7 adds retained command palette, quick select, and local launch menu modes. Active modes capture keyboard, character, pointer, wheel, and terminal mouse-reporting paths before the PTY while preserving damage-driven idle rendering.

Quick select labels visible HTTP(S) links plus compiled custom rules and rejects stale output/geometry/viewport/focus snapshots before copy/open side effects. Launch targets are declarative executable-plus-argv records; environment values are redacted in provenance, no shell wrapper or interpolation is inserted, and process spawn succeeds before pane topology commits. See [`docs/scripting.md`](docs/scripting.md) for syntax and hard limits.

The OpenGL backend keeps an authoritative RGBA offscreen frame image: damaged background pixels replace prior RGBA, while glyphs and overlays blend normally, and the complete image is presented each frame. Blur is routed through a capability-aware `BlurProvider`; providers report `active`, `disabled`, `unsupported`, `incompatible`, or `failed` and degrade without terminating. **The macOS AppKit, KDE/X11, and KDE/Wayland providers are experimental and compile-validated but have not yet completed real-compositor smoke testing.**

The GLFW frontend uses the current monitor content scale to rasterize text and scale window padding and chrome in framebuffer pixels. Moving the window between monitors rebuilds the glyph atlas at the new effective DPI. A GLFW-enabled `--doctor` reports the primary monitor scale and effective DPI; headless builds report that scale detection is unavailable.

Glyphs, including color emoji and shaped clusters, share at most two 2048 x 2048 RGBA atlas pages. ASCII is prewarmed; when both pages fill, CervTerm resets the atlas generation and rasterizes visible glyphs again on demand, keeping GPU texture memory bounded.

Monochrome text coverage is adjusted with `render.text_gamma` (default `1.15`) and `render.text_darken` (default `0.0`) for stronger antialiased edges and stems. Set them to `1.0` and `0.0` to restore the previous rendering. These settings affect monochrome text only; color emoji are left untouched.

Text uses unhinted, typeface-faithful rasterization by default (`render.text_raster = "go"`). On Windows, set `render.text_raster = "auto"` to opt into DirectWrite's grid-fitted hinting for Windows-Terminal-style uniform stems. Set `render.text_raster = "subpixel"` on any OS for a classic-macOS look: unhinted outlines with per-channel antialiasing designed for horizontal-RGB-stripe LCDs. Prefer `"go"` on rotated displays and OLED/PenTile panels. Color emoji and shaped clusters remain on their existing color or grayscale paths; subpixel rendering applies to individual monochrome glyphs, including fallback faces.

`font.family`, `font.size`, and `font.ligatures` remain compatible shorthand. Explicit v2 configuration can instead provide ordered `font.descriptors` with collection, weight, style, stretch, and augment/fixed matching; normal, bold, italic, and bold-italic resolve real faces when available and otherwise use cache-keyed synthetic modes. Unknown or unreadable candidates fail over deterministically, with embedded Go Mono as the final safe fallback.

`font.fallback` and bounded `font.rules` select one face for a whole cluster in authored-rule → primary → ordered-fallback → embedded order. Symbol classes cover emoji, CJK, Nerd Font PUA, Powerline, box drawing, braille, and symbols. Fallback is lazy: ordinary ASCII does not load fallback faces. `font.features` projects validated OpenType tags while the legacy `ligatures` boolean remains shorthand; all font resource fields require restart.

Fixed-grid controls `font.line_height`/`font.cell_width` (0.5–3.0) and `baseline_offset`/`glyph_offset_x`/`glyph_offset_y` (-64..64 px) change the shared cell canvas without changing logical per-glyph advances. Font environments retain at most 64 contexts, parsed font data remains bounded to 128 faces/256 MiB, and each context retains at most 8,192 negative results. `--safe-fonts` restores Go Mono and natural metrics. `--doctor` reports effective metrics, feature capability, concrete path-free primary style metadata, representative Powerline/Nerd/CJK/emoji/rule-tier selections, and capacity limits. Arbitrary active-terminal content selections and live cache counts remain unavailable in diagnostic-only mode.

## Experimental Kitty graphics opt-in

CervTerm implements a deliberately narrow, experimental Kitty graphics subset. It is disabled by default, restart-scoped, and rendered only by the existing GLFW/OpenGL frontend. The accepted direct-data actions are `t` (transmit), `T` (transmit and place), `p` (place), `d` (delete), and `q` (query). Accepted formats are RGB24 (`f=24`), RGBA32 (`f=32`), and PNG (`f=100`); `o=z` zlib compression applies only to raw RGB24/RGBA32 data.

Enable it in an explicit v2 config, then restart CervTerm:

```lua
return {
  config_version = 2,
  graphics = { kitty = { enabled = true } },
}
```

Transfers, decoded images, placements, replies, decode concurrency, and per-context GPU textures are bounded. Configurable limits may only lower the built-in caps. Protocol replies are fixed and value-free: `OK`, `EINVAL`, `ENOTSUP`, `ENOSPC`, `ETIME`, `ECANCELED`, `ENOENT`, and `EIO`. To roll back, set `graphics.kitty.enabled = false` and restart.

This is not a full Kitty conformance claim. Animation, external file/path/temporary-file/shared-memory transports, Unicode placeholders, Sixel, iTerm inline images, and non-OpenGL rendering are not supported. See [`docs/getting-started.md`](docs/getting-started.md) for exact caps and [`docs/troubleshooting.md`](docs/troubleshooting.md) for diagnostics.

## Known limitations

Box-drawing and block-element glyphs render procedurally for seamless joins at
any font or display scale. Diagonal box glyphs still use the configured font,
and rounded corners use square light-line joins rather than true arcs.

### Optional BiDi rendering

Set `render = { bidi = true }` to reorder each terminal row visually with the Unicode Bidirectional Algorithm. It is experimental and defaults to off. Terminal storage and selection remain logical: wrapped rows do not share paragraph context, mixed-direction selections may look discontiguous, and Arabic letters are not contextually joined across cells. Wide-cell pairs and combining marks remain attached while visual ordering is applied.

- Real primary style faces, deterministic synthetic fallback, lazy whole-cluster font fallback, feature projection, and metric offsets are implemented; broader installed-font qualification beyond Windows remains ongoing.
- DirectWrite shaping is implemented on Windows with Arabic/Indic/emoji smoke coverage; broader real-world fixture coverage is still growing.
- SVG text rasterization has basic layout support; real font selection/outline text remains future work.
- More redistributable color-font fixture subsets are needed for broad cross-platform emoji validation.
- A real MSYS2-built `vttest` startup/menu capture is automated; broader interactive `vttest` menu paths remain future coverage.
- Packaging has CI beta zip artifacts, tag-triggered GitHub release publishing, SHA256 checksums, GitHub provenance attestations, portable winget manifest templates, SVG icon source, generated Windows `.ico`, and CI `goversioninfo` resource embedding for Windows builds. Authenticode signing and MSI/WiX publishing are intentionally deferred.
