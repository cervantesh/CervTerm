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
