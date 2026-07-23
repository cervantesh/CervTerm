# Phase 13 Close-out Report

## Result

**Automated gates: PASS. Manual Windows/OpenGL visual matrix: UNRUN. Support disposition: experimental, default-off, subset-only.**

Phase 13 establishes a bounded protocol-neutral image model and direct-data Kitty subset without changing `core.Cell`, renderer selection, or cross-context GL ownership. The unrun GUI matrix prevents a stable platform/full-conformance claim; it does not weaken the automated safety, lifecycle, rollback, or disabled-path evidence.

## Exact gate evidence

Executed from repaired local `dev` (`9a66c9e` plus this documentation branch):

| Gate | Result |
|---|---|
| `go test ./... -count=1` | Pass |
| `go test -tags glfw ./... -count=1` | Pass |
| `go vet -unsafeptr=false ./...` | Pass |
| `go vet -unsafeptr=false -tags glfw ./...` | Pass |
| `go test -race ./... -count=1` | Pass |
| `go test -race -tags glfw ./internal/frontend/glfwgl ./internal/frontend/gpu ./internal/mux -count=1` | Pass |
| `go run ./scripts/check-maturity-gates.go` | Pass after behavior-preserving file splits (`d24ce73`) |
| `go run ./scripts/check-phase13-imports.go` | Pass |
| `git diff --check` | Pass after final documentation edits |
| `FuzzControlStringFraming`, 60 s | Pass |
| `FuzzParserAdvanceDoesNotPanic`, 60 s | Pass; rerun after RIS performance repair also passed |
| `FuzzKittyAdapter`, 60 s | Pass |
| `FuzzKittyDecode`, 60 s | Pass |

The complete test, tagged-test, vet, full-race, tagged-race, maturity, import, JSON-validation and diff-check block above was rerun **after** the final documentation, doctor, benchmark-harness split and image-aware whole-tab transfer test edits; all passed. The four 60-second fuzz targets had already passed against the final production paths, and the affected Kitty adapter fuzz was rerun after owner-driven expiry repair.

## Performance evidence

- Final text suite: snapshot **9,058 ns/op** (-7.65%), core reuse **3,029.5 ns/op** (-91.57%), parser **3,017.5 ns/op** (-91.64%); every benchmark remained 0 B/op and 0 allocs/op.
- Final GLFW carried `BenchmarkPhase13DisabledDraw`: **43,789 ns/op**, +1.60% versus 43,100.5 baseline, 0 B/op, 0 allocs/op.
- New actual image-dispatch `BenchmarkPhase13DisabledFrame`: introducing-slice median **7.222 ns/op**, worst 7.489 ns/op, 0 B/op, 0 allocs/op; initial budget <=8.0 ns/op. See `phase-13-disabled-frame.md`.
- Final control suite after the named RIS repair (`9842393`): overflow **+2.73%**, discard **+2.23%**, 0 B/op, 0 allocs/op. The measured production source matches the recorded digest; the authoritative gate passed.
- `core.Cell` remains 32 bytes through the baseline/maturity gates.

## Repair slices discovered by close-out

1. `d24ce73` split four production files over the 500-line maturity limit without behavior changes.
2. `9842393` removed a redundant parser-wide reset after ESC dispatch had already returned to ground; RIS terminal reset and parser reuse tests pass, and the control overflow benchmark returned inside budget.
3. `edadf0a` removed autonomous Store timers so transfer expiry and pending-ledger mutation occur through the Adapter/Mux owner loop; externally closed/reset transfers clear their deadlines without idle polling.

All repairs were independently reviewed and merged to local `dev` before close-out resumed.

## Architecture drift

Final re-audit after `edadf0a` found no remaining Phase 13 drift: `core.Cell` is unchanged; parser/core/mux publication and GL cache mutation remain owner-thread constrained; workers only decode candidates; resource handles are context-local; disabled behavior is nil; no external transport, renderer selection, Sixel, iTerm or animation path was introduced. Phase 14 can add bounded adapters against `termimage` without changing these ownership contracts.

## Operational contract

- Enable only with strict v2 `graphics.kitty.enabled=true`, then restart.
- Roll back with `graphics.kitty.enabled=false`, then restart.
- Disabled/default/v1 behavior creates no image budget, pane store, adapter, scheduler, worker, texture cache, retry deadline, draw mutation, or idle cadence.
- No support is claimed for external transports, animation, Unicode placeholders, Sixel, iTerm inline images, renderer selection, or full Kitty conformance.

## Remaining qualification boundary

Run and attach the Windows/OpenGL manual matrix in `docs/manual-verification.md` before considering any stable or platform-qualified support claim. Until then, `docs/parity-support-matrix.json` intentionally records `manual_qualification: "unrun"` and `support_claim: "subset_only"`.
