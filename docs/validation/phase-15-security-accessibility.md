# Phase 15 security and accessibility qualification

Date: 2026-07-23
Code candidate: `d468089`
Machine-readable local gate/fuzzer manifest: [`phase-15-security-manifest.json`](phase-15-security-manifest.json).

## Security automation

**Result: PASS.**

Executed on Windows 11 amd64 with Go 1.25.8:

```text
go test ./...
go vet -unsafeptr=false ./...
go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1
go run ./scripts/check-phase15-recovery.go -race
go test -race ./internal/accessibility ./internal/core ./internal/mux ./internal/termimage ./internal/workscheduler ./internal/kitty ./internal/sixel ./internal/itermimage ./internal/vt -count=1
govulncheck ./...
```

Results:

- all tests, vet and GLFW-tagged tests passed;
- config/reload, restore, logging, transient image activation/cache and diagnostic recovery race gates passed;
- focused accessibility/core/mux/image/scheduler/parser race suites passed;
- `govulncheck` found zero reachable vulnerabilities (four findings in imported packages and sixteen in required modules were unreachable from CervTerm code);
- package/preflight/smoke qualification rejected no required security or trust check.
- GitHub CodeQL run [`30048191712`](https://github.com/cervantesh/CervTerm/actions/runs/30048191712) passed for Actions, C/C++, Go, and Python at `d468089`; all four analyses reported zero results and the PR ref had zero open code-scanning alerts.

Every repository fuzzer ran for a two-second mutation interval after seed coverage:

- VT/control/public projection: parser no-panic, selected control framing, Sixel selection, OSC 1337 selection, public-output dual oracle and selected-envelope projection;
- image model: RGBA bounds, crop validation, delete selectors and store lifecycle;
- Kitty: adapter and decoder;
- Sixel: tokenizer fragmentation, adapter lifecycle and decoder;
- iTerm: scanner fragmentation, strict base64, adapter lifecycle and decoder;
- persistence: layout unmarshal.

No crash or new failing corpus entry was produced. Protocol adapters remain bounded, selected output remains redacted from public observers, and the accepted iTerm surface performs no external file/path/URL I/O.

The local and CodeQL results close P15-03. The separate required-CI/package merge gate remains part of release readiness.

## Accessibility automation

**Result: PASS for automated privacy/lifecycle scope.**

`go test ./internal/accessibility`, the full suite, the focused race suite, GLFW lifecycle tests, and the Phase 15 recovery gate passed. Coverage includes:

- visible-only immutable document projection;
- stable IDs/generations and stale callback rejection;
- cursor, selection, wide/combining, soft-wrap and BiDi bounds;
- alternate-screen privacy and no scrollback/URI leakage;
- event coalescing and lifecycle teardown;
- dormant UIA ABI/provider/router behavior and default-off production policy.

A real Windows GUI run with `accessibility.enabled=true`, `scope="visible"` started, rendered the shared protocol fixture, remained idle/responsive, and closed cleanly. No Narrator or NVDA interaction was executed in this Phase 15 run. Therefore UI Automation remains experimental/default-off and no assistive-technology support claim is made.

## Redaction and external-effect boundary

Recovery and diagnostics evidence is in [`phase-15-recovery-redaction.md`](phase-15-recovery-redaction.md). Capability output remains bounded and path-free; recovery classifications remain value-free; failed reload/restore/image transitions preserve the old state; notifications and image protocols stay behind explicit policy/configuration; and no new external file/network side effect was introduced.
