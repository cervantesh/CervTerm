# Phase 13.10 — Bounded Kitty decoders

Date: 2026-07-21
Branch: `feat/parity-phase-13-kitty-decode`
Base: local `dev` at `c602d0c`

## Scope

- Strict streaming base64 raw RGB24/RGBA32 decoding.
- Bounded zlib decoding with exact output, checksum/EOF, bomb and trailing-stream rejection.
- PNG config preflight before dimension-derived allocation, checked dimensions, conservative interlaced/16-bit scratch reservation, exact bounds/trailing checks, and straight-alpha RGBA conversion.
- Zero-copy sealed encoded-payload ownership transfer from `termimage` to synchronous decode jobs.
- Pane/process reservation of output and transient PNG memory before allocation.
- Cooperative cancellation, stale epoch/store rejection, exact rollback, and write-sealed immutable decode results.
- No goroutines, queues, parser/core/mux/frontend imports, publication, rendering, or external I/O.

## Security review

Independent reviews found and drove fixes for unreserved encoded copies, unaccounted base64 output, zlib trailing data, PNG Adam7/16-bit scratch under-accounting, and result immutability. Final verdict: **PASS**, no blocker or important finding.

## Verification

- `go test ./... -count=1` — pass.
- `go test -tags glfw ./... -count=1` — pass.
- `go test -race ./internal/kitty ./internal/termimage -count=1` — pass.
- `go vet -unsafeptr=false ./...` — pass.
- `go run ./scripts/check-phase13-imports.go` — pass.
- `go test ./internal/kitty -run '^$' -fuzz=FuzzKittyDecode -fuzztime=60s` — pass; 41,933,396 executions.

The inherited maturity gate failure for unchanged `internal/config/document.go` remains baseline-only.

## Deferred

Bounded scheduling, VT/mux wiring, acceptance deadlines, core publication, GPU upload/cache, and rendering remain later slices.
