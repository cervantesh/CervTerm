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

## Remaining CI check

The GitHub Actions workflow in `.github/workflows/ci.yml` is only verified after pushing this repo to GitHub and observing a green workflow run.
