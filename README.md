# CervTerm

CervTerm is an experimental GPU terminal emulator written in Go.

![CervTerm running a local Windows shell](docs/assets/cervterm-preview.png)

## Current status

CervTerm is not a finished daily-driver terminal yet, but it already includes:

- Windows ConPTY backend and Unix PTY backend behind build tags.
- GLFW/OpenGL frontend.
- Scrollback, alternate screen, resize reflow, selection/copy/paste, and bracketed paste.
- VT parsing for common cursor, erase, color, scroll-region, insert/delete, input-mode, mouse-mode, and title sequences.
- Keyboard encoding for navigation keys, F1-F12, and Ctrl/Alt/Shift modifiers.
- SGR mouse press/release/wheel/drag encoding, including modifiers.
- Lua config loading, Teal check/gen support, and `--print-default-config`.
- Renderer-neutral OpenType glyph backend with bitmap color fonts, broad COLRv1 paint/composite/variation support, SVG glyph extraction/rasterization, DirectWrite shaping smoke coverage, and shaped color cluster handling.
- Diagnostics logging via `--log-file` / `CERVTERM_LOG_FILE`, including panic stack capture.
- Parser fuzz smoke coverage, replay-style VT golden fixtures, and a Windows daily-driver smoke matrix for cmd.exe, git, pager, alternate-screen, resize/reflow, and longer-session paths.

- `--doctor` support diagnostics for config/log/environment reporting.
See:

- [`docs/product-roadmap.md`](docs/product-roadmap.md)
- [`docs/config-roadmap.md`](docs/config-roadmap.md)
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

## Build and test

```sh
go test ./...
go test -tags glfw ./internal/fontglyph ./internal/frontend/glfwgl ./cmd/cervterm -count=1
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

Generate a complete editable template with `--print-default-config`. A minimal example:

```lua
return {
  window = { width = 1100, height = 720, padding_x = 18, padding_y = 44 },
  font = { family = "Go Mono", size = 14 },
  shell = { program = "cmd.exe", args = {} },
}
```

## Teal config

`cervterm.tl` is checked and generated through the external `tl` command before CervTerm loads the generated Lua file. Lua remains the runtime target, so Teal is optional for users who prefer direct Lua config.

## Diagnostics logging

Runtime diagnostics are written to stderr and to a local log file by default. Override the location with `--log-file path/to/cervterm.log` or `CERVTERM_LOG_FILE`; use `--log-file -` to keep diagnostics on stderr only. Run `--doctor` to print the effective log path, config discovery state, environment hints, and support checklist. Unexpected panics are captured with a stack trace before CervTerm exits.

## Display scaling

The GLFW frontend uses the current monitor content scale to rasterize text and scale window padding and chrome in framebuffer pixels. Moving the window between monitors rebuilds the glyph atlas at the new effective DPI. A GLFW-enabled `--doctor` reports the primary monitor scale and effective DPI; headless builds report that scale detection is unavailable.

Glyphs, including color emoji and shaped clusters, share at most two 2048 x 2048 RGBA atlas pages. ASCII is prewarmed; when both pages fill, CervTerm resets the atlas generation and rasterizes visible glyphs again on demand, keeping GPU texture memory bounded.

Monochrome text coverage is adjusted with `render.text_gamma` (default `1.15`) and `render.text_darken` (default `0.0`) for stronger antialiased edges and stems. Set them to `1.0` and `0.0` to restore the previous rendering. These settings affect monochrome text only; color emoji are left untouched.

On Windows, `render.text_raster = "auto"` (the default) rasterizes primary-face monochrome glyphs with DirectWrite's hinted, grayscale coverage. Set it to `"go"` to restore the portable Go rasterizer. Color emoji, fallback faces, shaped clusters, and all text on non-Windows platforms continue to use the Go path.

`font.family` resolves installed `.ttf`, `.otf`, and `.ttc` faces from standard system and per-user font directories. Empty values and `Go Mono` keep the embedded font. An unknown or unreadable family logs a warning and safely falls back to Go Mono; `--doctor` shows the configured family and resolved files. Bold and italic variants are discovered for diagnostics, while rendering currently retains synthetic bold and italic transforms.

## Known limitations

Box-drawing and block-element glyphs render procedurally for seamless joins at
any font or display scale. Diagonal box glyphs still use the configured font,
and rounded corners use square light-line joins rather than true arcs.

### Optional BiDi rendering

Set `render = { bidi = true }` to reorder each terminal row visually with the Unicode Bidirectional Algorithm. It is experimental and defaults to off. Terminal storage and selection remain logical: wrapped rows do not share paragraph context, mixed-direction selections may look discontiguous, and Arabic letters are not contextually joined across cells. Wide-cell pairs and combining marks remain attached while visual ordering is applied.

- Bold and italic rendering in the GLFW frontend are synthesized (1px double draw for bold, quad shear for italic); real font variants remain future work.
- DirectWrite shaping is implemented on Windows with Arabic/Indic/emoji smoke coverage; broader real-world fixture coverage is still growing.
- SVG text rasterization has basic layout support; real font selection/outline text remains future work.
- More redistributable color-font fixture subsets are needed for broad cross-platform emoji validation.
- A real MSYS2-built `vttest` startup/menu capture is automated; broader interactive `vttest` menu paths remain future coverage.
- Packaging has CI beta zip artifacts, tag-triggered GitHub release publishing, SHA256 checksums, GitHub provenance attestations, portable winget manifest templates, SVG icon source, generated Windows `.ico`, and CI `goversioninfo` resource embedding for Windows builds. Authenticode signing and MSI/WiX publishing are intentionally deferred.
