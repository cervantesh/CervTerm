# Daily-driver Smoke Matrix

The daily-driver smoke matrix is a CI-safe Windows gate for common terminal workflows that are not well represented by unit tests alone. It records raw VT output through CervTerm's `--capture-vt` path and verifies markers in the captured streams.

Run locally from the repository root:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/daily-driver-smoke.ps1 -WorkDir dist/daily-driver-smoke -Version daily-smoke
```

Run against an already built or packaged executable:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/daily-driver-smoke.ps1 -CervTermExe dist\cervterm-<tag>-windows\cervterm.exe -WorkDir dist\daily-driver-smoke
```

## Covered workflows

| Case | Program | What it proves |
|---|---|---|
| `cmd-basic` | `cmd.exe` | ConPTY starts a Windows shell, emits text, and exits with markers captured. |
| `powershell-basic` | `powershell.exe` | PowerShell startup, output, location rendering, and simple pipeline output. |
| `git-log` | `git.exe --no-pager log --oneline` | Real Git output, colored output sequences, and repository command execution. |
| `pager-more` | `more.com` | Real pager behavior, screen-sized output, and `-- More --` prompt capture. |
| `alternate-screen` | PowerShell-emitted VT | Alternate-screen enter/exit sequences and content inside the alternate buffer. |
| `resize-reflow-40col` | PowerShell at 40 columns | Narrow PTY sizing and long-line capture for reflow regressions. |
| `resize-reflow-100col` | PowerShell at 100 columns | Wide PTY sizing and comparison coverage with the narrow capture. |
| `long-session` | PowerShell loop | Multi-line delayed output over a longer-running session. |

## Artifacts

Each case writes:

- `<case>.vt` raw terminal output;
- `<case>.log` CervTerm diagnostics log.

The script validates marker strings in the `.vt` files. It may warn about native process exit codes after ConPTY teardown, but still fails if the expected captures or markers are missing.

## Scope and limitations

This is a smoke gate, not a complete terminal certification suite. It increases confidence in common shell/pager/git/session paths before release. Full visual regression, screenshot diffing, and broader TUI application coverage remain separate future gates.
