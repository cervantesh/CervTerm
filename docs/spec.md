# cervterm MVP Spec

This document is the executable contract for the MVP. Implementation must follow tests; changes to behavior start here, then tests, then code.

## Non-goals for MVP

- No Fyne, no Gio, no Rust.
- No tabs, panes, SSH, serial, GPU glyph atlas optimization beyond a simple MVP frontend.
- No premature dirty-region optimizer, arena allocator, or custom memory pool until measurements justify it.

## Architecture constraints

1. `internal/core` is renderer-neutral and PTY-neutral.
2. `internal/vt` parses terminal bytes into `internal/core` operations.
3. `internal/render` exposes renderer-neutral frame snapshots.
4. `internal/pty` exposes a session interface; local PTY is only one domain.
5. `internal/frontend/glfwgl` is disposable and built only with `-tags glfw`.
6. Headless tests must pass with `go test ./...` without compiling GLFW/OpenGL.
7. Any future mux must route through pane/domain abstractions, not frontend state.

## MVP behavior

### Core terminal

- Maintains a fixed-size grid of cells.
- Each cell has rune, foreground, background, and bold attributes.
- Supports resize while preserving scrollback and reflowing auto-wrapped logical lines.
- Supports cursor movement with bounds clamping.
- Supports newline, carriage return, tab, backspace, clear screen, clear line, and scroll-up at bottom.
- Auto-wrap is enabled by default at the right edge; configurability is deferred.
- Resize reflow rejoins lines split by auto-wrap, but preserves explicit newline boundaries.

### VT parser

- Prints UTF-8 text.
- Preserves incomplete UTF-8 sequences across PTY read boundaries.
- Handles CR/LF/backspace/tab.
- Handles IND/NEL/RI (`ESC D/E/M`) and settable tab stops (`ESC H`, `CSI g/I/Z`).
- Handles CSI cursor movement: `A`, `B`, `C`, `D`, `H`, `f`.
- Handles cursor save/restore via `ESC 7`/`ESC 8` and `CSI s`/`CSI u`.
- Handles CSI erase modes: `J` modes 0/1/2/3 and `K` modes 0/1/2.
- Handles SGR basics: reset, bold, dim, blink state, ANSI 16 foreground/background including bright colors and default FG/BG resets.
- Handles extended SGR colors: 256-color `38;5;n`/`48;5;n` and truecolor `38;2;r;g;b`/`48;2;r;g;b`.
- Handles OSC 0/2 title updates with BEL or ST terminators.
- Answers DA1/DA2 and DSR/CPR reports (`CSI c`, `CSI > c`, `CSI 5 n`, `CSI 6 n`, `CSI ? 6 n`).
- Handles DECSET/DECRST bracketed paste mode: `CSI ?2004 h/l`.
- Handles DECSET/DECRST alternate screen mode: `CSI ?1049 h/l`, preserving the primary screen and scrollback.
- Handles DECOM origin mode, IRM insert mode, alternate screen variants `47`/`1047`/`1048`, DEC Special Graphics G0/G1 charsets, DECSCUSR cursor styles, mouse modes `1000`/`1002`/`1003`/`1004`/`1006`, OSC 52 clipboard writes, and OSC 7 working-directory URLs.

### Input encoding

- `internal/input` converts toolkit-neutral key and mouse events to terminal bytes.
- Printable runes encode as UTF-8.
- Enter, backspace, tab, escape, and arrow keys encode to common VT bytes.
- Ctrl-letter combinations encode to C0 control bytes, except Ctrl+V is reserved for paste by the frontend.
- Paste encoding wraps text with `CSI 200~`/`CSI 201~` when bracketed paste mode is active.
- Ctrl+Shift+C and Ctrl+Shift+V are standard clipboard shortcuts.
- Plain Ctrl+C remains available for terminal interrupt when there is no selection.

### Render snapshot

- Copies terminal state into a stable renderer-neutral frame.
- Reuses backing storage when dimensions do not grow.
- Does not alias terminal cells after capture.
- Steady-state capture benchmark must report `0 B/op` and `0 allocs/op`.

### Scrollback and selection

- Core keeps a bounded scrollback buffer and exposes a scrollable viewport.
- Render snapshots capture the current viewport, not only the bottom screen.
- Mouse wheel in the GLFW frontend scrolls the viewport.
- Mouse drag selects terminal cells in the current viewport.
- Ctrl+C copies selected text to the clipboard; when no selection exists it remains available for terminal interrupt input.

### Visual theme

- Default palette name is `cervterm dark`.
- Background and surface are dark, warm, and low glare.
- Foreground has high contrast without pure-white harshness.
- Accent color is stable and suitable for cursor/status details.
- Palette exposes exactly 16 tuned ANSI colors.

### Metrics

- Exposes verifiable runtime counters: frames, bytes read, heap allocation, mallocs, GC count, total pause, last pause.
- Parser/core benchmarks must report allocations with `-benchmem`.
- Reuse-vs-new benchmark must make allocation impact visible.

## Verification commands

```bash
go test ./...
go test ./internal/input
go test ./internal/vt -bench=. -benchmem
go test ./internal/render -bench=. -benchmem
go test ./internal/theme -bench=. -benchmem
GODEBUG=gctrace=1 go test ./internal/vt -bench=BenchmarkParserThroughput -benchmem
```

Optional GUI build/run:

```bash
go run -tags glfw ./cmd/cervterm
```

## Quality bar

- A failing spec test blocks further feature work.
- GUI is allowed to be minimal, but the default visual style must be refined: dark surface, soft foreground, tuned ANSI palette, subtle accent/status line.
- Measurements are not marketing claims; they must be reproducible with commands in this file.
