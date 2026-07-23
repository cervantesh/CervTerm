# Phase 15.1 — Support Matrix and Capability Doctor

Implementation commit: `c82f634`

## Result

`PASS` on Windows 11 `windows/amd64` with Go 1.25.8, using default and strict-v2 diagnostic configurations in both headless and `glfw` build modes.

## Delivered

- Corrected Phase 11 IME from planned to experimental Windows/default-off with the recorded J/C/K prerequisite skips and no broad support claim.
- Separated native Windows notification support from the supported semantic notification model.
- Added detached capability rows for advanced appearance, OSC 52, IME, accessibility, native notifications, Kitty, and Sixel/iTerm.
- Each row reports configured intent, static build/platform availability, runtime activation as `not-probed`, manual qualification, and support claim.
- Headless Windows builds report native frontend capabilities as unavailable rather than implying production activation.
- Added support-matrix consistency, default-off/exclusion maturity, static build-tag, section-structure, and value-free projection tests.
- Preserved existing ADR-0002 config provenance diagnostics and legacy doctor fields.

## Verification

- `go test ./cmd/cervterm -count=1` — PASS.
- `go test -tags glfw ./cmd/cervterm -count=1` — PASS.
- `go test ./... -count=1` — PASS.
- `go test -tags glfw ./... -count=1` — PASS.
- `go run ./scripts/check-maturity-gates.go` — PASS.
- `python -m json.tool docs/parity-support-matrix.json` — PASS.
- `git diff --check` — PASS.
- Independent correctness/privacy review — PASS after repairing headless Windows availability and doctor section nesting.

## Claim boundary

Static build availability does not establish live activation or manual platform qualification. Experimental features remain default-off and retain their existing support claims.
