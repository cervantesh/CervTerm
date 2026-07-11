# Daily-driver Smoke Matrix

The daily-driver smoke matrix is a CI-safe Windows gate for common terminal workflows that are not well represented by unit tests alone. It is implemented as a Go harness so CI does not depend on PowerShell script orchestration. The harness records raw VT output through CervTerm's `--capture-vt` path and verifies markers in the captured streams.

Run locally from the repository root:

```sh
go run ./scripts/daily-driver-smoke.go -workdir dist/daily-driver-smoke -version daily-smoke
```

Run against an already built or packaged executable:

```sh
go run ./scripts/daily-driver-smoke.go -cervterm dist/cervterm-<tag>-windows/cervterm.exe -workdir dist/daily-driver-smoke
```

## Covered workflows

| Case | Program | What it proves |
|---|---|---|
| `cmd-basic` | `cmd.exe` | ConPTY starts a Windows shell, emits text, and exits with markers captured. |
| `powershell-basic` | `powershell.exe` | PowerShell startup, output, location rendering, and simple pipeline output. |
| `git-log` | `git.exe --no-pager log --oneline` | Real Git output, colored output sequences, and repository command execution. |
| `pager-more` | `more.com` | Real pager behavior, screen-sized output, and `-- More --` prompt capture. |
| `alternate-screen` | Go helper executable | Alternate-screen enter/exit sequences and content inside the alternate buffer. |
| `resize-reflow-40col` | Go helper executable at 40 columns | Narrow PTY sizing and long-line capture for reflow regressions. |
| `resize-reflow-100col` | Go helper executable at 100 columns | Wide PTY sizing and comparison coverage with the narrow capture. |
| `long-session` | Go helper executable | Multi-line delayed output over a longer-running session. |

## Artifacts

Each case writes:

- `<case>.vt` raw terminal output;
- `<case>.log` CervTerm diagnostics log.

The Go harness validates marker strings in the `.vt` files. It may warn about native process exit codes after ConPTY teardown, but still fails if the expected captures or markers are missing.

## Scope and limitations

This is a smoke gate, not a complete terminal certification suite. It increases confidence in common shell/pager/git/session paths before release. Full visual regression, screenshot diffing, and broader TUI application coverage remain separate future gates.
