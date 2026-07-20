# Phase 11.9 — Windows native IME qualification

## Qualification record

- Date: `2026-07-20T02:53:54-04:00`
- Commit: `be4c3ffea9da80da57328e06a7af441a9e3e4c1c`
- Toolchain: `go1.25.8 windows/amd64`
- Reported OS: `Windows 10 Home`, version `2009`, build `26200`, `64-bit`
- Binary: local `dist/cervterm-phase11.exe`, version `0.1.0-dev`
- Native IME configuration: opt-in (`ime.enabled=true`); repository default remains `false`

## Installed input methods

`Get-WinUserLanguageList` reported only:

- `en-US`: `0409:00020409`, `0409:00000409`
- `es-ES`: `0C0A:0000040A`

No Japanese, Chinese, or Korean input method was installed. Installing OS language features changes machine state and was not performed implicitly.

## Automated and runtime evidence

| Check | Result | Evidence |
|---|---|---|
| Headless suite | PASS | `go test ./... -count=1` |
| GLFW suite | PASS | `go test -tags glfw ./... -count=1` |
| Required Windows tagged packages | PASS | `go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1` |
| Race suite | PASS | `go test -race ./... -count=1` |
| Vet and maturity gates | PASS | Both ordinary/tagged vet and maturity checks passed in Slice 11.8 |
| Enabled-config doctor | PASS | Reports authored/effective v2, `ime-enabled: true`, `windows-native-opt-in`, runtime activation unavailable in doctor mode |
| Enabled-config process smoke | PASS | Process remained running after three seconds and was then terminated by the harness; no startup/install crash |

## Required real-IME matrix

| Input method | Commit text | Preedit/target span | Candidate placement | focus/modal/pane cancellation | Result |
|---|---:|---:|---:|---:|---|
| Microsoft Japanese IME | — | — | — | — | SKIP — not installed |
| Microsoft Pinyin | — | — | — | — | SKIP — not installed |
| Microsoft Korean IME | — | — | — | — | SKIP — not installed |

## Decision

The implementation is automated-test qualified and startup-smoke qualified as an **experimental opt-in**. The real J/C/K matrix is incomplete, so it is not evidence for default enablement or a fully qualified native-IME support claim. Keep `ime.enabled=false` by default. Slice 11.10 must retain the default-off policy unless a later qualification run records all required real-IME rows as PASS.
