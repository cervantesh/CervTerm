# Phase 13 text-only and disabled-image baseline

Date: 2026-07-21
Slice: 13.0b
Production behavior baseline: `a5ae0c61ce94d18a507fb4d5116275f4124ff5da`; portable recapture revision: `4abc9f8`. The intervening commits add architecture documents and the baseline harness only, with no production behavior change.

## Environment

- OS: Microsoft Windows 11 Home `10.0.26200`, build `26200`, amd64.
- CPU: AMD Ryzen 9 7940HX with Radeon Graphics, 32 logical processors.
- Go: `go1.25.8 windows/amd64`.
- Host `GOMAXPROCS` is unset. Recorded benchmarks use `-cpu=1`; Go omits a CPU suffix when the sole CPU list is one.
- Each raw artifact embeds its capture commit plus a checkout-portable SHA-256 of the exact benchmark source/helper bundle. CRLF and LF checkouts hash identically; capture/comparison tooling may add suites without invalidating an earlier benchmark identity.

## Pinned invariants

- `TestCellSizeInvariant` requires `unsafe.Sizeof(core.Cell{}) == 32` exactly.
- `scripts/check-phase13-imports.go` rejects toolkit/frontend dependencies from the future `internal/termimage` package and rejects direct go-gl imports outside `internal/frontend`.
- All measured paths are text-only. There is no image package, store, worker, decoder, placement, texture, capability, support advertisement, or renderer selector in this baseline.

## Ten-sample benchmark baseline

The capture command owns the 5 s warm-up, fixed 2 s samples, single-P execution, ten repetitions, method/environment metadata, and exact harness digest:

```bash
go run ./scripts/capture-phase13-benchmark.go -suite text -out docs/validation/phase-13-baseline.txt
go run ./scripts/capture-phase13-benchmark.go -suite glfw -out docs/validation/phase-13-gl-baseline.txt
```

Median results computed by the authoritative self-contained comparison gate:

| Benchmark | Median ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkPhase13TextOnlyParser` | 36,085.0 | 0 | 0 |
| `BenchmarkPhase13TextOnlyCoreReuse` | 35,954.0 | 0 | 0 |
| `BenchmarkPhase13TextOnlySnapshot` | 9,808.0 | 0 | 0 |
| `BenchmarkPhase13DisabledDraw` | 43,100.5 | 0 | 0 |

The untagged and tagged raw Go benchmark outputs are tracked beside this document. Their metadata identifies Go/OS/architecture/CPU/method, package-qualified benchmark identity, production revision, and exact harness digest. Capture and validate candidate output with:

```bash
go run ./scripts/capture-phase13-benchmark.go -suite text -out phase13-candidate.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-baseline.txt phase13-candidate.txt
go run ./scripts/capture-phase13-benchmark.go -suite glfw -out phase13-gl-candidate.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-gl-baseline.txt phase13-gl-candidate.txt
```

The comparison command rejects mismatched metadata/harness/package/CPU/benchmark sets, malformed or non-finite records, and anything other than exactly ten samples. It fails a median `ns/op` regression above 3% and compares worst-sample `B/op`/`allocs/op`, so intermittent new allocations cannot hide behind a zero median. It invokes `benchstat` when already installed. Installation was declined for this slice; absence cannot weaken the authoritative self-contained gate.

`BenchmarkPhase13DisabledDraw` is deliberately a context-free text row-grid walk, not a claim about a frame-level image dispatch seam that does not exist yet. Slice 13.14 must introduce and baseline `BenchmarkPhase13DisabledFrame` at the actual pane/frame image integration boundary; the row-grid baseline remains as a separate regression signal.

## Disabled idle wake/frame baseline

The current runtime under `cmd/` and `internal/` is byte-identical to the synchronized Phase 12 process-evidence harness commit `e091a84`:

```bash
git diff --name-only e091a84..a5ae0c6 -- cmd internal
# no output
```

The carried-forward, three-process median from `docs/validation/phase-12-accessibility-closeout.md` and `phase-12-accessibility-process-metrics.csv` is therefore the Phase 13 pre-image baseline for the current production runtime:

| Configuration | Interval | Wakes | Frames |
| --- | ---: | ---: | ---: |
| Current runtime, accessibility/default-off | 3 s | 11 | 7 |

The corresponding explicit accessibility opt-in also produced 11 wakes and 7 frames, proving no activation-driven frame cadence in that evidence set. Phase 13 must not increase the default-off values or create an image-specific deadline when no image work exists.

Current scheduler-focused verification also passes:

```bash
go test -tags glfw ./internal/frontend/glfwgl -run 'Test(NextWake|ScrollbarFadeReturnsToIdle|ScrollbarAnimationWakeQuantizesAndReturnsIdle|AppPreeditMutationRequestsOneOnDemandFrame)$' -count=1
```

A fresh process capture remains available through a `-tags "glfw accessibilitymetrics"` build and `scripts/measure-accessibility-process.ps1` with `-Iterations 3 -IdleSeconds 3 -WarmupSeconds 1 -RuntimeMetricsDir <dir>` and v2 configs setting `accessibility.enabled=false` and `cursor.blink=false`. Slice 13.0b formally permits this carry-forward because the command/runtime tree is byte-identical and the source report already records executable/config hashes, readiness, CPU, heap, allocations, wakes, and frames.

## Acceptance for later slices

- `core.Cell` remains exactly 32 bytes.
- Matching disabled/text-only benchmark median regression is at most 3%.
- No measured disabled path gains `B/op` or `allocs/op`.
- Default-off idle wake/frame cadence does not increase.
- CPU/GPU/count reservations never exist while terminal images are disabled.
