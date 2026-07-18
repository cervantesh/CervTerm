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

## Remaining CI check

The GitHub Actions workflow in `.github/workflows/ci.yml` is only verified after pushing this repo to GitHub and observing a green workflow run.
