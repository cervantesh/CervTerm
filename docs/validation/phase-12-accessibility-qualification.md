# Phase 12 Windows accessibility qualification

Date: 2026-07-20
Branch: `feat/parity-phase-12-accessibility-opt-in`
Policy: experimental, restart-scoped, visible-only, default-off

## Automated evidence

| Gate | Result | Evidence |
| --- | --- | --- |
| Config/schema/Teal/provenance | PASS | `go test ./internal/config ./internal/script -count=1` |
| Detached projection/privacy/composition | PASS | `go test ./internal/accessibility -count=1` |
| GLFW capture/lifecycle | PASS | `go test -tags glfw ./internal/frontend/glfwgl -count=1` |
| Windows UIA ABI/provider/router | PASS | Windows-host tagged frontend suite |
| No config/state mutation | PASS | Focused filesystem test verifies evaluation/diff/live-merge leave the authored config byte- and mode-identical and create no accessibility state path |
| Windows 386 tagged link | SKIP | `GOARCH=386 CGO_ENABLED=1 go test -tags glfw ./internal/frontend/glfwgl -run '^$'` reached the linker; host has only x86_64 MinGW libraries (`-lgdi32`/`-lopengl32` unavailable). Pointer-width ABI invariants remain covered by tagged tests; no 386 support claim. |
| Full repository/package | PASS | `go test ./... -count=1`; `go vet -unsafeptr=false ./...`; maturity gates; tagged Windows build/tests; focused race suites; beta package, release preflight (39 PASS, one optional vttest warning), and installed-package smoke |

Automated coverage verifies strict v2 decoding, default-off legacy behavior, visible-only terminal capture, hidden-workspace redaction, bounded immutable documents, stable IDs/generations, Windows ABI/layout, provider lifetime, publication-before-event ordering, stale-generation rejection, panic containment, shared WndProc teardown ordering, and bounded non-sensitive diagnostics.

## Manual Narrator/NVDA matrix

| Scenario | Narrator | NVDA | Notes |
| --- | --- | --- | --- |
| ASCII/wide/combining/emoji/soft-wrap/BiDi | SKIP | SKIP | Screen-reader run not available in this implementation session. |
| Cursor and selection across split/zoom | SKIP | SKIP | Manual Windows desktop session required. |
| Hidden tabs/workspaces/windows and scrollback privacy | SKIP | SKIP | Manual Accessibility Insights inspection required. |
| Search/palette/launch/quick-select focus and edits | SKIP | SKIP | Manual screen-reader run required. |
| IME preedit/caret/target spans | SKIP | SKIP | Installed J/C/K IMEs and screen reader required. |
| Child window/layout restore/provider lifetime | SKIP | SKIP | Manual multi-window run required. |
| Rapid output/resize/scroll responsiveness | SKIP | SKIP | Manual performance observation required. |
| DPI changes with per-pane zoom and candidate/preedit geometry | SKIP | SKIP | Supervised mixed-DPI desktop session required. |
| Terminal resize, soft-wrap reflow and alternate-screen transitions | SKIP | SKIP | Manual screen-reader continuity run required. |
| Bell/notification announcement rate and body privacy | SKIP | SKIP | Manual event-rate observation with UIA client required. |
| Shutdown, close, restore and zero post-disconnect callbacks | SKIP | SKIP | Manual Accessibility Insights/UIA client observation required. |
| Default-off/no-provider regression | SKIP | SKIP | Manual Accessibility Insights inspection required. |

## Disposition

Automated implementation evidence may qualify the experimental opt-in for merge. Manual rows remain explicit SKIP, so this artifact makes no Narrator, NVDA, or general accessibility support claim and does not justify changing the default.
