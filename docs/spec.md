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

- Accepts bounded OSC 9 and OSC 777 `notify` requests as pane-local metadata with monotonic identity and explicit bounded-overflow events. Parsing never invokes a native notification, logs title/body content, or treats payload text as a command.
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

### Phase 13 experimental Kitty terminal graphics

This is the normative Phase 13 subset. It is available only through strict v2, restart-scoped `graphics.kitty.enabled=true`, remains experimental, and defaults to `false`. Enabled startup must fail closed if image budgets, mux ownership, the renderer image capability, or a projection/context cache cannot be prepared; no partially enabled mux or projection may be published.

Supported wire behavior is limited to Kitty `APC G` with:

- actions transmit (`a=t`), transmit-and-place (`a=T`), place (`a=p`), delete (`a=d`), and query (`a=q`);
- direct payload transport only (`t=d`, or omitted equivalent);
- raw RGB24 (`f=24`), raw RGBA32 (`f=32`), and PNG (`f=100`); zlib (`o=z`) only for raw RGB/RGBA;
- bounded base64 chunks (`m=0|1`), pane-scoped nonzero image IDs, and fixed value-free replies honoring `q=0|1|2`;
- current-cursor cell anchoring, paired cell spans (`c`/`r`), optional paired source crop (`x`,`y`,`w`,`h`), signed 16-bit z-order, and nonzero placement IDs; placement does not move the terminal cursor; and
- current-screen delete selectors all/all-and-resource (`d=a|A`), image/image-and-resource (`d=i|I`), and under-cursor/under-cursor-and-resource (`d=c|C`).

Unknown, duplicate, conflicting, malformed, cancelled, truncated, oversized, stale, late, or over-budget input must commit nothing. Replies are bounded fixed codes and never echo payload, pixels, paths, or raw metadata. Resource/placement replacement and deletion are owner-thread transactions: failure preserves the prior resource generation and placement set, and a success reply follows commit rather than precedes it.

The Phase 13 Kitty adapter excludes Sixel DCS and iTerm OSC 1337 from its own wire contract. Phase 14 supplies separate bounded opt-in adapters without widening Phase 13 ownership or hard caps. Phase 13 also excludes file/path/temporary-file/shared-memory transports, remote reads/writes, Kitty animation/frame composition, Unicode placeholders, unlisted Kitty keys/actions/delete selectors, protocol advertisement while disabled, renderer selection, and image bytes or handles in `core.Cell`.

Pane/process limits are bounded by ADR-0014 (8/32 pending transfers, 8/32 MiB encoded, 64/256 MiB decoded residency, 256/1,024 images, 1,024/4,096 placements, one/two decode workers). Each GL context additionally owns at most 512 textures and 256 MiB; configured values may only lower the applicable hard cap. Decode results are accepted only after pane/store/generation/deadline revalidation. Snapshots expose detached placement metadata, and a GL-context cache acquires detached exact-generation RGBA only on a miss.

With Kitty disabled, nil is normative: no image budget/store/adapter/scheduler/cache/deadline/draw/damage state is created; bounded APC/DCS input is consumed without image replies or text leakage; the frame seam performs no image mutation, allocation, redraw, or idle scheduling. Rollback is restart with `graphics.kitty.enabled=false`. This delivered subset is not a stable support claim: the Windows/OpenGL manual matrix in `docs/manual-verification.md` remains required, and unrun macOS/Linux GUI rows remain unqualified.

### Phase 14 experimental Sixel and iTerm inline images

Phase 14 is available only through independent strict-v2, restart-scoped `graphics.sixel.enabled=true` and `graphics.iterm.enabled=true` flags. Both remain experimental and default to `false`; their manual real-GUI qualification is UNRUN and their support claim is `none`. Either protocol may be enabled without Kitty or the other protocol. Any enabled image protocol creates one shared model, pane/process budgets, pane stores, FIFO decode scheduler, and one texture cache per OpenGL context; only enabled adapters are instantiated. With all three protocols disabled, the literal-nil/no-allocation/no-wake contract remains normative.

The exact 7-bit Sixel transport grammar is:

- introducer/preamble `ESC P q`, `ESC P 0q`, `ESC P 0;0q`, or `ESC P 0;0;0q`; C1 DCS is excluded;
- ST is the only successful terminator; CAN/SUB, overflow, reset, and EOF cancel atomically and discard without exposing payload as text;
- exactly one `"1;1;W;H` raster declaration is required before pixel output;
- accepted body tokens are `?`–`~`, `!N<char>` with `N=1..4096`, `#N`, `#N;2;R;G;B` with register `0..255` and RGB percentages `0..100`, `$`, and `-`; and
- HLS, second raster declarations, unknown forms, drawing outside the declared canvas, and results outside checked limits reject the whole image.

Sixel decoding is two-pass. Every syntax command and expanded output column consumes one of at most 4,194,304 operations. Width and height are each at most 4,096, the image is at most 16,777,216 pixels/64 MiB RGBA, and the cell span is `ceil(W/cellPixelWidth)` by `ceil(H/cellPixelHeight)` with each result in `1..256`. The detached 256-entry palette is captured from the pane; image definitions are local and unset pixels remain transparent.

The exact 7-bit iTerm transport grammar is `OSC 1337;File=<fields>:<strict padded base64>` terminated by BEL or ST; C1 OSC is excluded. Fields may appear in any order but must contain exactly one lexical `inline=1` and one positive decimal `size`. The body must be non-empty standard padded base64 that decodes to exactly `size` bytes and exactly one PNG with EOF. An optional `width=N` xor `height=N`, `N=1..256`, is cell-only; `preserveAspectRatio` may be absent or exactly `1`. Duplicate/unknown fields, `name`, whitespace, `inline=0`, omitted inline intent, auto/pixel/percent/two-axis/stretch sizing, non-PNG/trailing data, multipart forms, and file/path/URL/download/write or other external-I/O modes reject atomically.

Intrinsic iTerm sizing uses `Cols=ceil(Wi/cw)` and `Rows=ceil(Hi/ch)`. Width-only `C` uses `Rows=ceil(Hi*C*cw/(Wi*ch))`; height-only `R` uses `Cols=ceil(Wi*R*ch/(Hi*cw))`. Arithmetic is checked and every axis must remain in `1..256`; values are rejected rather than clipped.

The shared hard bounds remain: selected control-string chunks at most 16 KiB and a selected logical frame at most 256 KiB; 8 pending transfers/8 MiB encoded per pane and 32/32 MiB process-wide; at most 4,096 chunks and 10 seconds per transfer; 64 MiB decoded per image and pane, 256 MiB process-wide; 256/1,024 images and 1,024/4,096 placements per pane/process; one outstanding decode per pane, two process workers, FIFO queue capacity 32, and a queue-inclusive 250 ms acceptance deadline; and 512 textures/256 MiB per OpenGL context. Configurable graphics limits may only lower the exposed caps and apply to all enabled protocols.

Sixel/iTerm capture a canonical frame-termination anchor without moving the cursor, allocate monotonic non-reused internal image/placement IDs in `0x80000000..0xffffffff`, and atomically commit one create-only ephemeral resource plus placement. When the final placement retires through edit, erase, scroll, history eviction, ED3, reflow, alternate-screen exit, explicit deletion, reset, or close, the resource retires in the same owner transaction. Kitty wire IDs remain `1..0x7fffffff` and Kitty resources remain durable.

Sixel/iTerm emit no protocol replies and reserve no reply slots. Mux `PaneOutput` is a parser-coupled public projection: a selected frame for an enabled Kitty/Sixel/iTerm adapter is omitted across arbitrary PTY fragmentation, cancellation, overflow, reset, and EOF before Lua output callbacks receive it; disabled or unselected control strings remain public. Diagnostics expose only `Protocol`, fixed `Reason` (`invalid`, `unsupported`, `limit`, `timeout`, `cancelled`, `failed`, `stale`, or `busy`), `Count`, and `Duration`, never payload, pixels, metadata names, base64, or internal IDs.

Operational rollback is independent: set the affected `graphics.sixel.enabled` or `graphics.iterm.enabled` to `false` and restart. Activation/restore failures publish old-or-new state and close provisional resources in reverse acquisition order. Full or broad Sixel/iTerm conformance, animation, external I/O, cursor effects, Sixel scrolling/DECSDM, renderer selection, and non-OpenGL rendering remain outside this bounded subset.


### Scrollback and selection

- Core keeps a bounded scrollback buffer and exposes a scrollable viewport.
- Render snapshots capture the current viewport, not only the bottom screen.
- Mouse wheel in the GLFW frontend scrolls the viewport.
- Mouse drag selects terminal cells in the current viewport.
- Ctrl+C copies selected text to the clipboard; when no selection exists it remains available for terminal interrupt input.
- Typed `ScrollToPrompt(-1|1)` navigates bounded OSC 133/633 prompt history relative to the viewport top with generation-checked pane-local targeting.
- Typed `CopySemanticZone("input"|"output")` copies at most one MiB from the current semantic command cycle, preserving hard newlines while removing soft-wrap-only row breaks; copied text is never executed.
- Typed `SelectSemanticZone("input"|"output")` creates a pane-local selection only when the complete current-cycle range can be projected into one viewport; failures preserve the prior viewport and selection.

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

### Accessibility

- Accessibility state is a detached, bounded semantic projection; no native provider reads mutable terminal, mux, render, or App state.
- Text/range offsets follow logical grapheme order. Bounding rectangles follow rendered BiDi order, pane-local metrics, clipping, padding and zoom.
- Initial privacy scope exposes only the active visible viewport and focused modal/search input. Scrollback, hidden tabs/windows/workspaces, hyperlink targets and process metadata are excluded.
- A projection document is limited to 512 rows, 16,384 graphemes, 1 MiB UTF-8 and 256 nodes; truncation and generation staleness are explicit.
- Focus precedence is modal, search, then focused terminal pane. Repaint-only changes emit no semantic event; changes coalesce once per projection cycle.
- Windows UI Automation is available as a default-off experimental frontend adapter. Future macOS NSAccessibility and Linux AT-SPI adapters must consume the same pure document rather than widening core/mux contracts.
- Windows activation is restart-scoped and default-off until real Narrator/NVDA qualification passes.
- Enabled Windows projections coalesce semantic events, suppress repaint-only damage, redact hidden/minimized projections, gate native delivery on active UIA listeners and disconnect fail-closed without stopping terminal input/rendering.

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

### Bell policy

- Every BEL remains a monotonic pane-local core/mux event and fires one Lua `events.bell` callback; sink policy never coalesces observation.
- Strict v2 `bell` configuration reloads live. Effects default to disabled and may select one audible, visual, or taskbar sink with an `always` or `unfocused` focus rule.
- A bounded per-mode gate throttles only OS/visual effects. Visual expiry participates in the on-demand wake loop; native effects execute from frontend event dispatch on the locked OS thread.
- Windows audible mode uses the system message bell. Unsupported native audible adapters fail closed without dropping the observable bell event.

### Notification policy

- Strict v2 `notification` configuration reloads live and defaults to disabled explicit consent.
- A pure per-window gate rejects disabled, deferred, focus-suppressed, or rate-limited requests before any adapter call.
- Missing-window queues revoke request freshness, preventing delayed native effects after projection creation.
- Adapter/overflow diagnostics are bounded and redact all terminal-provided title/body content.
- Windows uses one projection-owned `Shell_NotifyIconW` balloon adapter with `NIF_REALTIME` and quiet-time respect; add/modify/delete failures roll back or fail closed without payload logging.
- Non-Windows notification adapters remain unavailable and fail closed.

## Native windows, workspaces, and saved layouts

- One process mux owns ordered named workspaces, native-window models, tabs, pane trees, and all local sessions/readers; native hosts are projections owned by the locked OS thread.
- Window/tab/pane transfers preserve identity and session ownership and perform no implicit respawn. Workspace switches hide/show hosts without suspending sessions.
- `layout_persistence.enabled` opts into the version 1 layout file; `layout_persistence.path` selects the file and otherwise uses the current-user config directory.
- Saved state contains only fresh-session topology/launch intent, bounds, focus/order, semantic CWD, and scalar appearance. It cannot contain environment maps, dedicated credential fields, PTY/process state, terminal content, scrollback, runtime IDs, or renderer selection.
- Restore creates fresh local sessions only. Complete native/mux preparation commits atomically; invalid or failed state falls back to one fresh window and remains non-authoritative.
- An unavailable saved color-scheme name rejects restore rather than silently projecting an unresolved palette.
