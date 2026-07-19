# cervterm MVP Spec

This document is the executable contract for the MVP. Implementation must follow tests; changes to behavior start here, then tests, then code.

## Non-goals for MVP

- No Fyne, no Gio, no Rust.
- No SSH/serial/WSL/local-domain abstraction, daemon, detach/reattach, or persistence of live pane processes. Visible tabs, local windows, and layout-only workspaces are post-MVP roadmap work.
- No premature dirty-region optimizer, arena allocator, or custom memory pool until measurements justify it.

## Architecture constraints

1. `internal/core` is renderer-neutral and PTY-neutral.
2. `internal/vt` parses terminal bytes into `internal/core` operations.
3. `internal/render` exposes renderer-neutral frame snapshots.
4. `internal/pty` exposes a local session transport interface; no domain abstraction is planned.
5. `internal/frontend/glfwgl` is disposable and built only with `-tags glfw`.
6. Headless tests must pass with `go test ./...` without compiling GLFW/OpenGL.
7. The native in-process mux owns pane identity, split topology, focus, geometry, lifecycle and one independent session aggregate per leaf; the frontend only projects and routes.
8. PTY readers only enqueue pane-addressed records; terminal/parser/mux mutation remains serialized on the GLFW main thread.
9. Every pane render is confined by the backend-neutral renderer clip stack.

10. OpenGL through GLFW remains the sole supported renderer; configuration must not expose renderer/backend selection.
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
- Handles DECOM origin mode, IRM insert mode, alternate screen variants `47`/`1047`/`1048`, DEC Special Graphics G0/G1 charsets, DECSCUSR cursor styles, mouse modes `1000`/`1002`/`1003`/`1004`/`1006`, OSC 4/10/11 palette set/query and OSC 104/110/111 reset, OSC 52 clipboard writes, OSC 7 working-directory URLs, bounded pane-local OSC 8 hyperlink metadata, and bounded OSC 133/633 prompt/input/output semantic metadata. Parsing never opens a URI; opening requires a fresh explicit click or quick-select acceptance and centralized absolute HTTP(S) policy.

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
- Typed `ScrollToPrompt(-1|1)` navigates bounded OSC 133/633 prompt history relative to the viewport top with generation-checked pane-local targeting.

### Native panes

- One implicit tab contains a binary split tree with one-pixel draggable dividers and process-local ratios.
- `Alt+Shift+=` splits the focused pane left/right; `Alt+Shift+-` splits top/bottom.
- `Alt+Arrow` changes focused pane; `Ctrl+Shift+W` closes the focused pane and collapses its parent split.
- Left-dragging a divider resizes its adjacent subtrees live, preserves every descendant's 10x3 minimum, and settles ConPTY sizes once on release.
- Lua bindings take precedence over built-in pane bindings.
- Each pane has independent PTY/parser/core/snapshot, scrollback, selection, search, links and mouse-report state.
- Keyboard/paste targets the focused pane; pointer operations first hit-test a pane and translate to pane-local cells.
- PTY-origin Lua output/title/CWD/bell callbacks remain bound to the originating pane for the callback duration.
- Multi-pane frames repaint fully for correctness; incremental pane damage is deferred.
- Font zoom is pane-local: keyboard/wheel/reset target the focused pane, sibling panes retain their size/grid, and new splits inherit the source pane's zoom.
- All active font sizes share one fixed two-page atlas; pane focus never clears atlas pages.

### Font descriptors, fallback, shaping, and metrics

- Unversioned/v1 configuration keeps `font.family`, `font.size`, and `font.ligatures` compatibility. Explicit v2 additionally supports ordered `font.descriptors`, `font.fallback`, bounded `font.rules`, OpenType `font.features`, and fixed-grid metric projection.
- Descriptor selection is deterministic across family, collection selector, weight, style, stretch, and augment/fixed attribute mode. Normal, bold, italic, and bold-italic use real faces when available and only then use keyed synthetic modes.
- Fallback is lazy and whole-cluster: authored rule, primary, ordered fallback, then embedded Go Mono. ASCII must not trigger fallback I/O; losing candidates release cache pins.
- `line_height` and `cell_width` are bounded to 0.5–3.0; baseline/glyph offsets are bounded to -64..64 pixels. They change cell canvases and ink placement without content-dependent advances.
- Font environments retain at most 64 contexts; the parsed cache retains at most 128 faces/256 MiB; each context retains at most 8,192 negative results. All font fields remain restart-scoped.
- `--safe-fonts` restores embedded Go Mono, clears descriptors/fallback/rules/features, and restores natural 1/1/0/0/0 metrics.


### Phase 5 appearance and native window controls

- `padding.left/right/top/bottom` are independent logical insets used consistently for DPI-scaled layout, grid sizing, clipping, and hit testing.
- Text and composed-background opacity are independent; whole-window opacity/blur compatibility validation remains authoritative.
- Backgrounds are ordered, bounded solid/linear-gradient/image layers with constrained decode dimensions/bytes, cache ownership, and deterministic fit/alignment.
- Scrollbar visibility is `always`, `hover`, `scrolling`, or `never`; stable gutter prevents PTY resize, and animation FPS bounds fade wakeups.
- `render.max_fps` is an optional presentation cap. It does not replace vsync or create continuous idle rendering.
- Initial `window.rows`/`window.cols` and native `decorations`/`titlebar` are startup/recreation controls with platform capability fallback.
- Renderer selection is not a Phase 5 field and remains excluded.

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
go run ./scripts/capture-parity-baseline.go -count 3
```

Optional GUI build/run:

```bash
go run -tags glfw ./cmd/cervterm
```

## Quality bar

- A failing spec test blocks further feature work.
- GUI is allowed to be minimal, but the default visual style must be refined: dark surface, soft foreground, tuned ANSI palette, subtle accent/status line.
- Measurements are not marketing claims; they must be reproducible with commands in this file.
- `docs/parity-support-matrix.json` is the machine-readable feature-status contract; update it with every support-state change.
- The phased post-MVP contract is `docs/wezterm-parity-roadmap.md`.

### Tab bar

`tab_bar.mode` is `multiple` (default), `always`, or `hidden`; `position` is `top` or `bottom`. Height, tab widths, and horizontal padding are bounded and reload live as one validated configuration. The retained bar reserves authoritative window geometry, keeps the active tab visible under overflow, and routes add/close/tab hits before terminal mouse input. The default one-tab window reserves no bar pixels.

## Native windows, workspaces, and saved layouts

- One process mux owns ordered named workspaces, native-window models, tabs, pane trees, and all local sessions/readers; native hosts are projections owned by the locked OS thread.
- Window/tab/pane transfers preserve identity and session ownership and perform no implicit respawn. Workspace switches hide/show hosts without suspending sessions.
- `layout_persistence.enabled` opts into the version 1 layout file; `layout_persistence.path` selects the file and otherwise uses the current-user config directory.
- Saved state contains only fresh-session topology/launch intent, bounds, focus/order, semantic CWD, and scalar appearance. It cannot contain environment maps, dedicated credential fields, PTY/process state, terminal content, scrollback, runtime IDs, or renderer selection.
- Restore creates fresh local sessions only. Complete native/mux preparation commits atomically; invalid or failed state falls back to one fresh window and remains non-authoritative.
- An unavailable saved color-scheme name rejects restore rather than silently projecting an unresolved palette.
