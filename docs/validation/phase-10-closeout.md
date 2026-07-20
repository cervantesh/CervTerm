# Phase 10 Validation — Shell Semantics and Trusted Effects

## Scope and disposition

Phase 10 is complete for bounded OSC 8 hyperlinks, OSC 133/633 shell semantic metadata and actions, strict bell sinks, bounded OSC 9/777 notification requests, default-off notification consent, and the Windows native notification adapter. Renderer selection and domains remain excluded. Native notification effects on macOS/Linux are not claimed.

ADR-0008 is accepted: terminal parsing creates metadata/requests only; every OS effect is gated on the frontend OS thread behind a fakeable adapter.

## Cross-slice contract

- The shared OSC collector is bounded and all-or-nothing. Malformed, truncated, invalid-control, invalid-UTF-8, and oversized inputs mutate no Phase 10 metadata.
- Hyperlink and semantic identity survive wide/combining cells, row mutation, scrollback, reflow, resize, and primary/alternate isolation without growing the 32-byte cell.
- OSC 8 output never opens automatically. A safe absolute HTTP(S) URI requires a fresh explicit press/release on the same stable identity; unsafe schemes, malformed authorities, credentials, and stale regions fail closed.
- Semantic prompt/input/output history is bounded, detached, and origin-pane addressed. Prompt navigation and input/output copy/selection preserve modal and viewport safety.
- Every BEL remains monotonic through core/mux/Lua. Only optional audible/visual/taskbar sinks are focus-filtered or throttled.
- OSC 9/777 metadata never invokes a native API directly. Consent defaults off; accepted effects require freshness, focus eligibility, and bounded rate policy. Missing-projection queues revoke freshness.
- Windows notification icons are projection-owned. Add/modify/delete is transactional; live consent withdrawal cleans up; failed deletion retains ownership for retry. Diagnostics are coalesced and omit title/body.

## Automated evidence

Focused cross-slice qualification adds `internal/frontend/glfwgl/testdata/phase10/powershell-osc633.vt` and `phase10_qualification_test.go`. The raw chunked fixture traverses parser -> terminal -> mux -> frontend, verifies OSC 633 input/output actions, proves notifications are default-off, requires explicit safe-link activation, then verifies explicitly consented notification routing. A malicious/truncated PTY case proves unsafe links and malformed notifications cannot reach external-effect adapters.

Existing focused suites additionally cover:

- `internal/vt/parser_osc8_test.go`, `parser_semantic_test.go`, and `parser_notification_test.go`: BEL/ST, chunking, malformed/oversized atomicity, and payload non-retention.
- `internal/core/hyperlink_test.go`, `semantic_test.go`, and `semantic_history_test.go`: bounded identity/history, erase/overwrite, scrollback, reflow/resize, reset, and alternate-screen isolation.
- render/mux snapshot tests: detached bounded projection and row damage.
- action/codec/script/frontend tests: stable origin targeting, prompt navigation, copy/select behavior, modal/viewport safety, and Lua/Teal surfaces.
- link, bell, and notification policy tests: explicit/fresh activation, safe schemes/authority, focus/rate/default-off behavior, lossless callbacks, stale queue handling, redacted diagnostics, and Windows adapter lifecycle.

Qualification gates:

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race ./... -count=1
go run ./scripts/check-maturity-gates.go
python -m json.tool docs/parity-support-matrix.json
```

Executed on Windows on 2026-07-20 from base `ebd8c19` plus the Phase 10 close-out candidate:

| Gate | Result | Evidence |
|---|---|---|
| focused Phase 10 GLFW integration | Pass | PowerShell OSC 633 fixture, default-off/consented notification routing, explicit safe link activation, consented malformed-notification rejection, unsafe-link denial, and truncated-link non-projection |
| `go test ./... -count=1` | Pass | all default packages |
| `go test -tags glfw ./... -count=1` | Pass | all packages including Windows GLFW adapter and cross-slice fixture |
| default and GLFW `go vet` | Pass | no findings |
| `go test -race ./... -count=1` | Pass | all default packages |
| maturity gates | Pass | only documented pre-existing font-file exceptions |
| support matrix JSON and `git diff --check` | Pass | valid JSON and clean diff |

Windows CI now executes the GLFW frontend/cmd tests rather than compile-only coverage. Linux CI remains headless.

## Platform qualification

| Platform | Automated | Manual/native disposition |
|---|---|---|
| Windows | Local default, GLFW, vet, race, policy, ABI/lifecycle and fixture tests; CI runs default/vet/fuzz plus GLFW frontend/cmd tests | Manual balloon/focus/quiet-time/tray cleanup matrix remains required before a release support claim |
| Linux | Headless default tests, vet, fuzz smoke and package build | GUI bell/link behavior skipped; native notifications unavailable and fail closed |
| macOS | No CI runner in this repository | GUI/native behavior skipped; native notifications unavailable and fail closed |

The manual matrix is recorded in `docs/manual-verification.md#phase-10-shell-semantics-and-trusted-effects-qualification`. An unrun GUI check is a skip, never a pass.

## Rollback

Disable notification and bell effect policies first while retaining harmless metadata. Revert native adapters before policy gates, actions before semantic storage, and parsers last. Existing OSC 7/52, detected HTTP(S) links, BEL callbacks, terminal rendering, and one-pane behavior remain compatibility baselines.
