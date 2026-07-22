# CervTerm Manual Verification

Use this checklist after building `dist/cervterm.exe`.

## Build

```sh
go test ./...
go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -run '^$' -count=0
go build -tags glfw -o dist/cervterm.exe ./cmd/cervterm
```

## Smoke launch

```sh
./dist/cervterm.exe
./dist/cervterm.exe --version
./dist/cervterm.exe --config path/to/cervterm.lua
```

## Interactive checks

Inside CervTerm, verify:

- cmd.exe starts and accepts input.
- `cmd.exe` starts when configured as the shell.
- `vim` insert/delete and cursor movement do not corrupt the screen.
- `less` and `git log` use alternate screen/scrollback correctly.
- Resize preserves visible content and scrollback.
- Copy/paste and bracketed paste work.
- Mouse-aware TUIs receive clicks/wheel when mouse reporting is enabled.
- CJK text such as `A好B` aligns cursor and cells.
- Combining text such as `é` does not advance the cursor twice.

## Native pane checks

- `Alt+Shift+=` creates a right-hand pane; `Alt+Shift+-` creates a lower pane.
- Nested row/column splits preserve non-overlapping one-pixel dividers and clip italic, wide, combining and color glyphs at pane edges.
- `Alt+Arrow` moves focus geometrically and the accent border/window title follow the focused pane.
- Hover each divider and verify the resize cursor matches its axis; left-drag both root and nested dividers in both directions.
- Drag to each minimum-size boundary and verify it stops without collapsing, overlapping, selecting text, opening links or sending mouse reports to a TUI.
- Release after a long drag and verify each shell/TUI receives its settled size once without duplicated banners or scrollback corruption.
- Input, paste, scrollback, search, selection, links and mouse-aware TUIs operate only in the intended pane.
- Run cmd.exe in one pane and PowerShell in another; verify output and parser replies never cross sessions.
- Give two panes visibly different zoom levels with `Ctrl++`/`Ctrl+-` or Ctrl+wheel; verify only the focused pane changes, `Ctrl+0` resets only that pane, mouse hit-testing remains aligned, and switching focus does not flash or rebuild text.
- Closing one pane with `Ctrl+Shift+W` leaves siblings running and collapses the tree; closing the final pane closes the window.
- Exit a shell without closing its pane; its final screen remains visible until explicit close.
- Trigger title, OSC 7 CWD and bell changes in a background pane; Lua callbacks target that pane while the window title remains focused-pane-derived.
- Repeat split/close/resize loops and confirm no orphan process, ConPTY handle, reader goroutine or stale hover/selection state remains.

## Phase 4 font qualification

Windows 11 `windows/amd64` qualification at merge commit `03efcc5` used the packaged `0.0.0-phase4-local` build:

- `--doctor` resolved JetBrainsMono Nerd Font `Regular`/`Bold`/`Italic`/`Bold Italic` (not ExtraLight) with `synthetic=none`, DirectWrite shaping capability, natural/projected metrics `11x28/21`, representative rule-tier family metadata, and no emitted font paths.
- Windows package preflight reported 39 pass, 0 required failures, and the expected missing-vttest warning; installed-package and daily-driver smokes passed. The known ConPTY capture exit `0xc0000374` was accepted only after required artifacts/markers validated.
- Linux is CI-qualified for headless build/tests only. macOS and Linux GUI font behavior remain unqualified; no GUI/platform support claim is implied.

| Windows visual check | Result | Evidence |
|---|---|---|
| punctuation/ligature centering (`->`, `=>`, `!=`, `===`) | Pass | [font qualification frame](assets/phase4-font-qualification-windows.png) |
| primary/CJK/emoji baseline alignment | Pass | [font qualification frame](assets/phase4-font-qualification-windows.png) |
| Powerline and Nerd Font PUA | Pass | [font qualification frame](assets/phase4-font-qualification-windows.png) |
| pane-local independent zoom and sibling stability | Pass | [mixed-size pane frame](assets/phase4-pane-zoom-windows.png) |
| fixed-grid hit testing, cursor split, clipping | Pass (automated) | GLFW metric/atlas/pane tests |
| cross-monitor DPI transition | Skip | no second monitor was available; automated DPI/context identity tests passed |

Manual recheck recipe: use an installed JetBrainsMono Nerd Font primary, `Microsoft YaHei UI` CJK rule, and `Segoe UI Emoji` emoji rule; print the strings above, then verify real style selection with `--doctor`, glyph alignment, cursor splits inside ligatures, pane-local zoom, and no sibling flash.

## Phase 5 appearance and window qualification

Automated qualification covers default geometry/pixels, per-side insets, independent text/background alpha, layered background budgets and reload rollback, OSC 11 precedence, scrollbar modes and idle cadence, `render.max_fps`, and checked initial-grid planning.

| Platform check | Result | Evidence |
|---|---|---|
| Windows layered backgrounds, alpha and OSC 11 reset | Pass (automated); GUI recheck required | background/GLFW golden and transaction tests |
| Windows scrollbar modes, overlay/stable gutter and idle | Pass (automated); GUI recheck required | scrollbar policy/fake-clock tests |
| Windows system/none decorations and dark/system titlebar | Pass (automated creation plan); GUI recheck required | window-plan and platform titlebar tests |
| Windows initial rows/cols first PTY grid | Pass (automated planning); installed GUI recheck required | checked startup plan tests |
| Linux headless | Pass | default/headless CI-compatible tests |
| Linux/macOS GUI native appearance | Skip | no GUI runner available; no support claim implied |
| Renderer selection | Excluded | Phase 5 does not expose or change renderer selection |

## Phase 6 input and topology qualification

Use a temporary config containing one legacy flat callback, a leader that enters both one-shot and persistent tables, exact mouse bindings, and all three topology actions. Then verify:

- The legacy flat `keys` callback and typed root binding both execute; fixed reload and active search still take precedence.
- Leader repeat does not leak bytes. `Escape`, mismatch, timeout, reload, and focus loss cancel without leaking the cancelling key; table actions retain the pane where the sequence began.
- One-shot tables exit after a match; persistent tables remain active and refresh their timeout.
- Mouse bindings require exact event/button/modifier/click-count matches. A matched press owns drag/release on the original pane without duplicate selection, link, divider, scrollbar, or PTY delivery.
- With terminal mouse reporting enabled, reports win; holding Shift preserves the reporting override and allows the configured/UI route.
- Resize/swap/move use the geometric neighbor in each direction. Resize respects the 2-column/2-row floor, swap keeps focus in the visual slot, and move keeps focus on the moved pane.
- Attempt each topology action at an outer edge and minimum bound; an error notice appears and pane order, focus, geometry, and PTY sizes remain unchanged.

## Phase 7 retained UX qualification

Use temporary bindings for `ActivateCommandPalette`, `ActivateQuickSelect`, and `ActivateLaunchMenu`, then verify:

- Each mode captures printable keys, navigation, mouse buttons/drag, wheel, and terminal mouse-reporting input without PTY leakage; Escape restores the opening pane.
- Command palette filtering is rune-safe, excludes unavailable/unlabeled actions, preserves state on error, and closes after successful execution.
- Quick select labels visible HTTP(S) links and custom copy rules; output, resize, viewport movement, or focus change cancels stale labels before side effects.
- Launch menu passes arguments containing spaces/quotes/metacharacters as separate argv entries, honors cwd on Windows/Unix, and applies configured environment overrides without duplicate keys.
- A launch failure leaves query, selection, focus and pane count unchanged; success creates exactly one pane and closes the mode.
- Leave each mode unchanged while the terminal is idle and confirm no continuous frame presentation or CPU wake loop.

Automated qualification covers bounds, composition/provenance, reload rollback, Unicode coordinates, stale generations, URL validation, clipboard content, environment normalization, Windows cwd plumbing, spawn atomicity, modal input capture, and zero-frame retained damage. Real Windows GUI interaction remains a manual recheck; macOS/Linux GUI checks are skipped without support claims.

## Remaining CI check

The GitHub Actions workflow in `.github/workflows/ci.yml` is only verified after pushing this repo to GitHub and observing a green workflow run.

## Phase 8 visible tabs qualification

Automated qualification is recorded at `docs/validation/phase-8-visible-tabs.md`. On Windows, additionally verify:

- The default `multiple` policy leaves a one-tab window pixel/grid compatible; opening the second tab reserves exactly one top or bottom strip and resizes the PTY once.
- Tab, close and add hits never select terminal text, activate links, drag dividers, scroll the terminal or emit terminal mouse reports; an active modal still captures first.
- Narrow the window until tabs overflow and activate first/middle/last tabs; the active tab remains visible and Unicode/emoji titles do not split clusters.
- Background output adds one activity badge without continuous frames; activating the tab clears it.
- Close an inactive running tab, then rename/reorder/move a pane before accepting: the stale confirmation must reject. An unchanged confirmation closes exactly once.
- Exit every pane in a tab and click close: it closes directly. Closing the final tab closes the native window only after `WindowTabsEmpty`.
- Move panes between nested tabs repeatedly and verify shell processes, scrollback, parser state and input ownership remain continuous with no extra spawn/close.

macOS and Linux GUI tab-bar behavior remains manually unqualified; Linux headless CI covers model/config/frontend compilation and tests.

## Phase 9 windows, workspaces, and persistence qualification

Enable `layout_persistence`, use two monitors/DPI scales when available, and verify:

- Create several native windows, tabs, nested splits, and named workspaces; move whole tabs and panes between windows and confirm the original shells, scrollback, parser state, zoom, and input ownership remain continuous.
- Switch workspaces repeatedly. Inactive windows hide without stopping output; returning restores remembered window/tab/pane focus.
- Move/resize windows across monitors, change split ratios/tab order/focus/CWD, then close CervTerm normally. Restart and verify bounds recovery, ordering/topology and scalar appearance while every pane starts a fresh local process.
- Inspect the JSON state and confirm it contains no environment maps, dedicated credential fields, runtime IDs, PTY/process handles, terminal cells, scrollback, or renderer selection. Treat program arguments as visible trusted launch data.
- Corrupt the file, use a future version, or select an unavailable saved scheme. Each case must keep state non-authoritative and open one usable fresh window without leaked hidden windows or processes.
- Remove a monitor or make the saved CWD unavailable. Valid topology must remain while bounds recover to a current monitor and CWD uses the documented fallback.
- Inject/induce a launch failure in one restored pane and confirm the entire candidate closes before the normal fresh-window fallback appears.

Automated evidence is recorded at `docs/validation/phase-9-windows-workspaces-layout-persistence.md`. Windows GUI multi-monitor verification remains manual; Linux is headless-qualified and macOS/Linux GUI behavior is not claimed.

## Phase 10 shell semantics and trusted effects qualification

Use a shell integration that emits OSC 133 or OSC 633, plus a temporary v2 config that explicitly enables each desired bell/notification sink. Verify:

- Prompt navigation, input/output copy, and viewport-safe selection follow the current shell cycle after scrollback, resize, soft wrap, and alternate-screen entry/exit.
- OSC 8 and detected HTTP(S) links never open on output alone; a press/release on the same fresh region opens once. `file:`, `javascript:`, credentials, malformed authorities, and stale regions remain blocked.
- Every BEL invokes the Lua callback even when native effects are disabled, focused, or throttled. Audible/visual/taskbar modes obey their configured focus and throttle policy.
- With notifications at their default, OSC 9/777 produces no native effect. After explicit enablement, unfocused/focus-always and rate settings behave as configured; changing `enabled` back to false removes the projection-owned notification icon.
- On Windows, accepted notifications use the native notification-area balloon, respect quiet time, truncate Unicode safely, and remove their icon on window close/rollback. Adapter errors must not reveal title/body.

Automated cross-slice evidence is recorded at `docs/validation/phase-10-closeout.md`. Windows native API behavior still requires the manual checks above. Linux is headless-qualified; macOS and Linux GUI link/bell/notification behavior is explicitly skipped and no native notification support is claimed.

## Phase 11 native IME qualification

Native Windows IME support is experimental and restart-scoped. Set `config_version = 2` and `ime = { enabled = true }`, restart CervTerm, and retain the default `false` on machines that have not completed this matrix.

For each installed Microsoft Japanese, Pinyin, and Korean IME:

- Compose and commit ASCII, BMP CJK and supplementary-plane text; verify exactly one PTY write and no duplicate GLFW echo.
- Exercise converted and unconverted target spans, moving the IME cursor through surrogate pairs and grapheme clusters.
- Verify the candidate window follows terminal, search and command-palette carets across padding, splits, independent pane zoom, DPI changes and RTL text.
- Change pane/tab/window/modal focus during preedit; verify cancellation targets the captured activation and never writes preedit to the PTY.
- Resize, scroll, zoom, switch workspaces, create/close child windows and restore a saved multi-window layout while composing.
- Trigger malformed/oversized input through the fake test harness; verify cancellation, bounded diagnostics and continued GLFW fallback.
- Disable `ime.enabled`, restart, and verify no native subclass/candidate activity and unchanged legacy GLFW character input.

Record exact Windows build, CervTerm commit, Go architecture, IME names/versions and PASS/SKIP/FAIL for every row in `docs/validation/phase-11-ime-qualification.md`. A missing J/C/K IME is SKIP, never PASS, and cannot justify default enablement.

## Phase 12 Windows UI Automation qualification

Accessibility remains experimental, restart-scoped, visible-only, and default-off. On Windows 11, test both Narrator and a current NVDA release with `accessibility = { enabled = true, scope = "visible" }`:

- Read simple ASCII, wide CJK, emoji/combining clusters, soft wraps, RTL/BiDi rows, cursor and selected text in one pane and multiple independently zoomed panes.
- Enter and leave alternate screen, scroll beyond the visible viewport, switch tabs/workspaces, and hide/show windows; verify inactive/hidden content and offscreen scrollback are never announced or inspectable.
- Open search, command palette, launch menu and quick select; edit queries and move modal focus. Verify stable controls, focus, whole-text replacement and no stale terminal document.
- Enable IME separately, compose Japanese/Chinese/Korean preedit, and verify text, caret and target spans follow the active target without exposing PTY-bound content prematurely.
- Inspect the UIA tree with Accessibility Insights: verify no OSC 8 URI, shell command, environment value, native handle, provider token, raw pane title, or unbounded user label appears.
- Create/close child windows and restore a saved layout. Verify fresh provider identity, monotonic generations, no stale window roots, no duplicate controls, and no focus leakage.
- Exercise rapid output/resize/scroll and forced test-hook publication failure. Verify coalesced updates, responsive input/rendering, one bounded fallback notice, and no native callbacks after close.
- Restart with `accessibility.enabled = false`; verify CervTerm installs no UIA provider and legacy terminal behavior is unchanged.

Record Windows build, CervTerm commit, architecture, screen-reader/version, config, and PASS/SKIP/FAIL per row in `docs/validation/phase-12-accessibility-qualification.md`. Automated ABI/projection/privacy tests do not replace this matrix and do not establish a support claim.

Automated performance, process-sample, gate and tool-availability evidence is recorded in `docs/validation/phase-12-accessibility-closeout.md`. Its assistive-technology rows are explicit SKIP and do not replace this manual matrix.

## Phase 13 experimental Kitty images — Windows/OpenGL qualification

Phase 13 is delivered as a restart-scoped, default-off experiment, not as a stable support claim. Use a strict v2 temporary config with `graphics.kitty.enabled=true`, keep all configured limits at or below their built-in caps, restart for every enabled/disabled comparison, and record the exact Windows build, GPU/driver, OpenGL vendor/renderer/version, Go architecture, CervTerm commit, test application/fixture and config. A row not personally exercised is **UNRUN**, never PASS.

Before enabling, run the normal build gates above plus the focused Phase 13 automated suites. Those suites cover bounded framing/decoding, atomic model lifecycle, detached snapshots, mux ownership, cache/GL fakes, clipping/layer order and disabled-frame allocation/idle behavior; they do not replace a real Windows/OpenGL run.

| Windows 11 / OpenGL manual check | Result | Required evidence |
|---|---|---|
| Default-off launch, Kitty query and malformed/oversized APC remain visually inert; ordinary UTF-8 input, idle cadence and text rendering are unchanged | UNRUN | config, query bytes/tool, screenshot or trace, idle observation |
| Opt-in direct RGB24, RGBA32 and PNG transmit-and-place render with correct colors/alpha; raw zlib and multi-APC chunking complete once | UNRUN | fixture hashes/commands and screenshots |
| Separate transmit then place, query reply policy (`q=0/1/2`), delete all/image/under-cursor variants and generation replacement behave deterministically | UNRUN | captured replies plus before/after screenshots |
| Cell spans, source crop and negative versus nonnegative z-order render in the documented layers; cursor and application overlays remain above images | UNRUN | screenshots for crop and both z bands |
| Images remain clipped to their pane through splits, independent zoom, divider drag, scrollback, erase, insert/delete line/char, resize/reflow and alternate-screen enter/exit | UNRUN | scripted sequence and screenshots/video |
| Moving a pane/tab between native windows and workspaces preserves CPU image identity but uploads independently in the destination GL context; closing the source window does not invalidate the moved image | UNRUN | two-window trace/screenshots and cache counters if available |
| Repeated image replacement plus window create/close exercises cache bounds, deterministic omission/eviction and bounded upload retry without input stalls or cross-context texture reuse | UNRUN | GPU/debug trace, responsiveness notes, final resource counts |
| Timeout, cancellation, malformed base64/PNG/zlib, over-budget payload and close-during-decode produce no partial placement/success reply and leave the prior image usable | UNRUN | fixture command, reply capture and before/after state |
| RIS, pane close, failed child-window/restore preparation and application shutdown release pending work, CPU reservations and context textures without crash or stale redraw | UNRUN | failure-injection or reproducible sequence and teardown trace |

### Phase 13 platform disposition

| Platform row | Result | Claim boundary |
|---|---|---|
| Windows 11 `windows/amd64`, real GLFW/OpenGL GUI | UNRUN | Automated Windows tests/benchmarks exist, but no Phase 13.16 real-GUI matrix was recorded; Kitty remains experimental/default-off. |
| Windows 11 `windows/arm64`, real GLFW/OpenGL GUI | UNRUN | No run or support claim recorded. |
| Linux, headless/non-GL tests | UNRUN for this closeout | Prior slice evidence is not a fresh Phase 13.16 platform run. |
| Linux, real GLFW/OpenGL GUI | UNRUN | No GUI qualification or Kitty support claim recorded. |
| macOS, real GLFW/OpenGL GUI | UNRUN | No GUI qualification or Kitty support claim recorded. |

Record any future run in a dedicated validation artifact before changing these rows, `docs/parity-support-matrix.json`, release notes, or the default. Partial completion stays experimental/default-off; a failed security, ownership, rollback or context-isolation row blocks a support/default-on proposal.
