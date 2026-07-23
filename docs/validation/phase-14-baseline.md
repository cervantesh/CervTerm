# Phase 14 disabled-path and authority baseline

Date: 2026-07-23
Slice: 14.0
Production baseline: `be30c587dac9422fe500294e83380a41d26b6a42`

## Scope

This slice adds architecture authority, static invariant/import checks and a checkout-portable benchmark capture. It changes no production file under `cmd/` or `internal/`.

Pinned invariants:

- `core.Cell` remains exactly 32 bytes.
- Phase 14 protocol leaves may not depend on core/render/mux/frontend/config/VT or filesystem/network/process/unsafe facilities.
- Go-GL/GLFW remains frontend-only; renderer selection is excluded.
- All image protocols disabled remains the Phase 13 literal-nil path.
- Existing hard limits remain unchanged: pending transfers 8/pane and 32/process, encoded residency 8/32 MiB, decoded residency 64/256 MiB, 4,096 px per axis, 16,777,216 pixels, two workers and 250 ms late-commit rejection.

## Reproducible capture

The orchestrator captures the four established Phase 13 suites plus the actual disabled image-frame dispatch seam into a temporary directory outside the checkout, so every embedded dirty-tree observation is taken before any output is published. The underlying metadata/harness digest is CRLF/LF portable.

```bash
go run ./scripts/check-phase14-imports.go
go run ./scripts/capture-phase14-baselines.go
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-baseline.txt docs/validation/phase-14-text-baseline.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-control-string-baseline.txt docs/validation/phase-14-control-baseline.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-image-store-baseline.txt docs/validation/phase-14-store-baseline.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-gl-baseline.txt docs/validation/phase-14-glfw-baseline.txt
go run ./scripts/check-phase14-frame-baseline.go docs/validation/phase-14-glfwframe-baseline.txt
```

The five artifacts were captured from clean authority/harness revision `b1eec13` and amended into the same slice. Every metadata preamble reports `working_tree_dirty=false`, the same production commit, fixed single-P two-second ten-sample commands and a checkout-portable harness digest. The `glfwframe` extension adds a new suite without changing the identity of earlier Phase 13 suites.

## Ten-sample results

| Suite | Benchmark median ns/op | Worst B/op | Worst allocs/op |
|---|---:|---:|---:|
| text | snapshot 8,988; core reuse 2,961; parser 2,969 | 0 | 0 |
| control | discard 149,872; overflow 1,757,069.5 | 0 | 0 |
| store | reserve/release 43.77; acquire miss 0.97; begin/cancel 112.40 | 64 / 0 / 192 | 1 / 0 / 2 |
| glfw disabled row-grid draw | 43,273 | 0 | 0 |
| glfw actual disabled image frame | 7.13 | 0 | 0 |

## Idle evidence

`git diff --name-only be30c58..HEAD -- cmd internal` must remain empty for this slice. The Phase 13 close-out therefore remains the process-level idle baseline: default-off creates no image budget, pane store, parser adapter, scheduler, worker, texture cache, retry deadline, draw mutation or idle cadence. Phase 14 Slice 14.15 must recapture the actual any-protocol-disabled integration boundary.

## Acceptance

- Architecture authority and ADR index are internally consistent.
- Static check passes and detects future forbidden Phase 14 protocol imports.
- The four inherited suites compare against their Phase 13 baselines; the actual disabled-frame artifact passes its fixed <=8 ns/op, 0 B/op and 0 allocs/op budget. All metadata is clean and has exactly ten samples.
- Production diff from `be30c58` is empty.
- Full untagged/tagged tests and vet, full race, maturity, Phase 13/14 import checks and `git diff --check` pass.
