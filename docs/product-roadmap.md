# CervTerm Product Roadmap

This roadmap captures the path from the current CervTerm prototype toward a minimum daily-driver terminal comparable in ambition to Alacritty, while borrowing configuration and UX lessons from WezTerm and Kitty where appropriate.

## Current baseline

CervTerm already has:

- Local Windows PTY via ConPTY.
- GLFW/OpenGL frontend.
- Renderer-neutral snapshots.
- Scrollback with scrollable viewport.
- Resize reflow that preserves scrollback and reflows auto-wrapped logical lines.
- Word wrap enabled by default.
- Selection, copy, paste, bracketed paste.
- Alternate screen buffer support.
- Cursor save/restore.
- Correct cursor hiding while viewing scrollback.
- Basic VT parser with CSI cursor movement, erase modes, SGR ANSI/bright/256/truecolor, OSC title.
- Benchmarks for parser/render hot paths.
- Product rename to CervTerm.

## Four-phase roadmap

### Phase 1 — Daily-driver terminal correctness

Goal: make common shells and TUI applications behave correctly before investing heavily in visual polish.

Priority work:

1. VT scroll regions: `CSI t;b r`.
2. Insert/delete character and line operations:
   - `ICH` / `CSI @`
   - `DCH` / `CSI P`
   - `IL` / `CSI L`
   - `DL` / `CSI M`
3. Additional cursor movement:
   - `G`, `d`, `E`, `F`, `S`, `T`.
4. Cursor visibility mode:
   - `CSI ?25 h/l`.
5. Autowrap mode:
   - `CSI ?7 h/l`, default enabled.
6. Application cursor/keypad modes.
7. Mouse reporting:
   - normal tracking `?1000`
   - button-event tracking `?1002`
   - SGR mouse `?1006`
8. Complete keyboard encoding:
   - Home/End/PageUp/PageDown/Insert/Delete
   - F1–F12
   - Ctrl/Alt/Shift modified arrows and navigation keys.
9. Wide character support:
   - CJK width 2.
   - combining marks baseline.
   - no cursor desync with wide text.

Success criteria:

- `vim`, `less`, `top`, `git log`, PowerShell, and `cmd` are usable without major visual corruption.
- Scrollback and alternate screen remain isolated correctly.
- Cursor position stays accurate across wraps, resize, wide chars, and scrollback.
- Parser/render hot path remains allocation-free in steady state where practical.

### Phase 2 — Appearance and font quality

Goal: replace the prototype bitmap font path with a real configurable terminal rendering stack.

Priority work:

1. Configurable font family and size.
2. Load system fonts.
3. TTF/OTF rasterization.
4. Dynamic glyph atlas.
5. Antialiasing.
6. DPI scaling.
7. Bold/italic faces or synthetic styles.
8. Font fallback.
9. Cursor shapes:
   - block
   - underline
   - beam
10. Theme-driven colors:
   - foreground/background
   - selection
   - cursor
   - ANSI 16 palette
   - indexed overrides.

Success criteria:

- User can choose a normal programming font such as JetBrains Mono, Consolas, or CaskaydiaCove.
- Glyphs, cursor, selection, and cell backgrounds remain aligned.
- Text rendering is visibly acceptable for daily use.
- Theme colors can be changed without editing Go code.

### Phase 3 — Robustness, configuration, and compatibility

Goal: make CervTerm configurable, testable, and less fragile.

Priority work:

1. Add `internal/config` with Go config structs, defaults, and validation.
2. Support both configuration authoring modes:
   - `cervterm.lua` for direct Lua configuration.
   - `cervterm.tl` for typed Teal configuration that validates/compiles to Lua.
3. Implement config discovery:
   - explicit `--config`
   - portable mode beside `cervterm.exe`
   - `%APPDATA%/cervterm/`
   - `$XDG_CONFIG_HOME/cervterm/`
   - `$HOME/.config/cervterm/`.
4. Add live reload for safe settings:
   - colors
   - padding
   - scroll multiplier
   - cursor style
   - font size once font backend is ready.
5. Add parser fuzzing.
6. Add VT golden tests from recordings.
7. Add vttest-based compatibility checklist.
8. Add robust PTY lifecycle handling:
   - process exit detection
   - clean close
   - resize error handling.
9. Add Unix PTY support.

Success criteria:

- Invalid config reports actionable errors and falls back safely.
- Lua config works without Teal installed.
- Teal config catches schema/type mistakes before runtime.
- CI can run unit tests, parser fuzz smoke tests, and benchmarks.
- Windows and at least one Unix-like platform are supported.

See also: [`docs/config-roadmap.md`](config-roadmap.md).

### Phase 4 — Productization and release

Goal: make CervTerm installable, releasable, and understandable to users.

Priority work:

1. Create icon and app metadata.
2. Build `cervterm.exe` release artifact.
3. Add Windows installer or zip distribution.
4. Add GitHub Actions CI:
   - tests
   - benchmarks/smoke checks
   - Windows build
   - Linux build once Unix PTY exists.
5. Add release notes/changelog.
6. Add README with:
   - screenshots
   - installation
   - configuration examples
   - known limitations.
7. Add default config template generation.
8. Add crash/error logging.
9. Add version command or `--version`.

Success criteria:

- A user can download CervTerm and run it without compiling.
- A user can find/edit config with documented examples.
- Releases are reproducible through CI.
- Core limitations are documented rather than surprising.

## Implementation guardrails

- Keep source files under 500 lines whenever practical.
- If a touched source file is already over 500 lines, split it by responsibility and bring it below 500 lines before adding more feature work, unless there is a documented reason not to.
- Prefer focused files such as editing, scrolling/screen state, modes, resize, input, and rendering helpers over large catch-all modules.
- Include a line-count check for touched source files during verification.

## Implementation plan

### Milestone 1: VT correctness slice

1. Implement scroll regions.
2. Add tests for full-screen scroll vs region scroll.
3. Implement insert/delete character and line.
4. Add tests using representative screen grids.
5. Implement cursor visibility mode and use it in render snapshot.
6. Run `go test ./...`, VT benchmarks, render benchmarks.

Deliverable:

- TUI apps relying on scroll regions and insert/delete operations behave significantly better.

### Milestone 2: Input and mouse slice

1. Expand key enum in `internal/input`.
2. Encode navigation/function keys.
3. Add modified key support.
4. Track application cursor/keypad modes in core/parser.
5. Implement mouse reporting modes.
6. Route mouse wheel either to app or scrollback depending on mode.
7. Add tests for encoded bytes.

Deliverable:

- `vim`, shells, pagers, and mouse-aware TUIs receive more correct input.

### Milestone 3: Unicode slice

1. Add width calculation helper.
2. Store wide-cell continuation state or equivalent.
3. Update `PutRune` for width 0/1/2 behavior.
4. Add combining mark handling.
5. Add tests for CJK, combining accents, and resize/wrap interactions.

Deliverable:

- Cursor and text remain aligned for common non-ASCII content.

### Milestone 4: Font/render slice

1. Choose font rasterization library.
2. Add configurable font descriptor model.
3. Load default system monospace font.
4. Replace static basicfont atlas with dynamic atlas.
5. Preserve renderer hot-path benchmarks.
6. Add visual smoke tests where feasible.

Deliverable:

- CervTerm looks like a real terminal rather than a bitmap-font prototype.

### Milestone 5: Config slice

1. Add `internal/config` structs and defaults.
2. Add config validation tests.
3. Add Lua loader.
4. Add Teal validation/compile flow.
5. Add docs and generated sample config.
6. Wire config into window/font/colors/scrolling/shell.

Deliverable:

- CervTerm can be configured via Lua or typed Teal.

### Milestone 6: Packaging slice

1. Add CLI flags: `--config`, `--version`, maybe `--print-default-config`.
2. Add app icon/version metadata.
3. Add CI release build.
4. Add README screenshots and install steps.
5. Add changelog.

Deliverable:

- CervTerm can be distributed and used by someone other than the developer.

## Near-term recommendation

The next engineering task should be **scroll regions plus insert/delete line/character operations**. That work is more important than configuration or packaging because it improves real terminal compatibility immediately and reduces visible breakage in TUI applications.

After that, implement full keyboard/mouse encoding, then start the font rendering replacement.
