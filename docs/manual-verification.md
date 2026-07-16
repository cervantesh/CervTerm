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
- Input, paste, scrollback, search, selection, links and mouse-aware TUIs operate only in the intended pane.
- Run cmd.exe in one pane and PowerShell in another; verify output and parser replies never cross sessions.
- Resize and font zoom update every pane without duplicated banners or scrollback corruption.
- Closing one pane with `Ctrl+Shift+W` leaves siblings running and collapses the tree; closing the final pane closes the window.
- Exit a shell without closing its pane; its final screen remains visible until explicit close.
- Trigger title, OSC 7 CWD and bell changes in a background pane; Lua callbacks target that pane while the window title remains focused-pane-derived.
- Repeat split/close/resize loops and confirm no orphan process, ConPTY handle, reader goroutine or stale hover/selection state remains.

## Remaining CI check

The GitHub Actions workflow in `.github/workflows/ci.yml` is only verified after pushing this repo to GitHub and observing a green workflow run.
