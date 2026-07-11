# VT/vttest Capture Notes

CervTerm now has a replay harness for deterministic VT byte streams:

- raw recording: `internal/vt/testdata/<name>.vt`
- expected viewport text: `internal/vt/testdata/<name>.golden`
- replay test: `TestParserGoldenRecordings` in `internal/vt/parser_test.go`

## Current captured fixtures

| Fixture | Coverage | Status |
| --- | --- | --- |
| `fullscreen-region` | fullscreen text, scroll region, region scroll, region reset, cursor-home overwrite | automated in `go test ./internal/vt` |
| `powershell-ansi-smoke` | real ConPTY capture with clear-screen/private-mode noise, OSC title, and ANSI-colored `RED` text | automated in `go test ./internal/vt` |
| `vttest-startup` | real MSYS2-built `vttest` menu capture through CervTerm ConPTY recorder | automated in `go test ./internal/vt` |

## Adding a capture

1. Record raw PTY output directly with CervTerm:

   ```sh
   go run ./cmd/cervterm --capture-vt internal/vt/testdata/<name>.vt --capture-program vttest --capture-rows 24 --capture-cols 80
   ```

   Add `--capture-arg value` repeatedly when the program needs arguments. Add `--capture-timeout 30s` for bounded non-interactive captures.

2. Add the expected `Terminal.PlainText()` result as `internal/vt/testdata/<name>.golden`.
3. Add the fixture name and terminal dimensions to `TestParserGoldenRecordings`.
4. Run:

   ```sh
   go test ./internal/vt -run TestParserGoldenRecordings -count=1 -v
   ```

## Manual vttest workflow

`--capture-vt` starts a real PTY/ConPTY session, mirrors child output to the console, and writes the exact PTY output bytes to the requested `.vt` file. That makes it the preferred path for release-quality `vttest` fixtures. The helper script at `scripts/capture-vt-session.ps1` remains a fallback for simple command captures, but PowerShell text pipelines can normalize bytes and should not be treated as authoritative for tricky escape-sequence cases.

On Windows, build `vttest` in a real MSYS2/POSIX environment rather than Strawberry/MinGW, because the latter lacks the termios/ioctl headers required by `vttest`:

```powershell
# one-time MSYS2 setup, run in an elevated or normal MSYS2 shell:
# pacman -S --needed base-devel gcc make ncurses-devel

powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-vttest-msys2.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/capture-vttest.ps1 -VttestExe dist/tools/vttest-msys2/install/bin/vttest.exe -Output internal/vt/testdata/vttest-manual.vt
```

The first release-quality real `vttest` capture is committed as `vttest-startup`, generated from an MSYS2-built `vttest.exe` through CervTerm's `--capture-vt` ConPTY recorder. Future work is to add more representative menu paths/screens as stable replay fixtures with `.golden` snapshots.
