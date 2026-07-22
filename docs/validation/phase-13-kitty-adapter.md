# Phase 13.9 — Bounded Kitty command adapter

Date: 2026-07-21
Branch: `feat/parity-phase-13-kitty-adapter`
Base: local `dev` at `fe8f7d5`

## Scope

- New protocol-only `internal/kitty` package for APC `G` command parsing.
- Supported actions: transmit, transmit-and-place, place, delete, and query.
- Direct transport only; RGB24, RGBA32, PNG, and raw zlib metadata only. No decoding or terminal/render activation.
- Strict field/action matrices, duplicate/unknown/conflict rejection, bounded header and payload retention.
- Pane/process encoded-transfer ownership through `termimage.Store`.
- 8 pending transfers per pane, 32 per process, 4,096 logical frames per transfer, and 10-second sliding silence expiry.
- Fixed finite redacted replies with normal/errors-only/all quiet policy.
- Atomic rejection, discard-until-APC-terminator behavior, exact rollback, transfer sealing, and idempotent close/expiry.

## Security disposition

Independent review initially found and drove fixes for:

- expired transfer sealing;
- split timeout clocks;
- delete selector/image conflicts;
- oversized APC suffix reinterpretation;
- continuation quiet/action drift;
- insufficient stateful fuzz invariants.

Final independent blocker review: **PASS**, with no remaining blocker or important finding.

## Verification

- `go test ./... -count=1` — pass.
- `go test -tags glfw ./... -count=1` — pass.
- `go test -race ./internal/kitty ./internal/termimage -count=1` — pass.
- `go vet -unsafeptr=false ./...` — pass.
- `go vet -unsafeptr=false -tags glfw ./...` — pass.
- `go run ./scripts/check-phase13-imports.go` — pass, including the new Kitty dependency boundary.
- `go test ./internal/kitty -run '^$' -fuzz=FuzzKittyAdapter -fuzztime=60s` — pass; 54,902,843 executions in the final recorded run.
- Store baseline comparison — pass with 336 B/op and 4 allocs/op retained for transfer begin/cancel.

`go run ./scripts/check-maturity-gates.go` still reports the inherited `internal/config/document.go` 515-line violation present unchanged on base `fe8f7d5`; this slice does not modify that file or worsen the baseline.

## Deferred

Base64/zlib/PNG decoding, worker scheduling, VT sink installation, mux reply reservation wiring, core publication, GPU caches, renderer presentation, and protocol advertisement remain deferred.
