# Phase 13 bounded control-string baseline

Date: 2026-07-21
Slice: 13.1
Base revision: `1650bc6`

## Contract under measurement

- APC (`ESC _`) and DCS (`ESC P`) use dedicated payload, pending-ESC, overflow-discard, and discard-pending-ESC states.
- Payload budget is 256 KiB; borrowed chunks are at most 16 KiB.
- ST completes; CAN/SUB, parser reset, and EOF cancel exactly once.
- The first over-budget byte emits one overflow cancellation, after which bytes are discarded through ST/CAN/SUB.
- A nil sink consumes and discards the complete frame.
- OSC framing remains separate and unchanged.

## Ten-sample first-result baseline

The portable capture tool owns a 5 s warm-up, fixed 2 s samples, `-cpu=1`, ten repetitions, method/environment metadata, a normalized benchmark-harness digest, and a separate measured-production-source digest. Later implementation changes may be compared without pretending the measured source is identical.

```bash
go run ./scripts/capture-phase13-benchmark.go -suite control -out docs/validation/phase-13-control-string-baseline.txt
go run ./scripts/capture-phase13-benchmark.go -suite control -out phase13-control-candidate.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-control-string-baseline.txt phase13-control-candidate.txt
```

| Benchmark | Median ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkPhase13ControlStringDiscard` | 156,049.0 | 0 | 0 |
| `BenchmarkPhase13ControlStringOverflow` | 1,774,993.5 | 0 | 0 |

The isolated final control and pre-existing text-only candidate captures both passed the mandatory 3% median threshold and worst-sample allocation gate. Every measured path remained at zero bytes and zero allocations per operation.

## Qualification discoveries

- Independent review found and closed repeated-ESC introducer leakage and exact-cap overlapping-ST discard recovery.
- Binary framing fuzz now covers ESC, CAN/SUB, malformed bytes, arbitrary split points, and a separate no-leak oracle.
- The mandatory existing parser fuzz found an oversized CSI parameter that could overflow into a high-cost tab count. CSI values now saturate at the supported `uint16` geometry maximum (65,535), forward/backward tab traversal stops at terminal boundaries, and the minimized corpus is retained under `internal/vt/testdata/fuzz/`.
- Control state is allocated only when a non-nil sink is installed, keeping the default text parser on its compact zero-allocation path.
- Final untagged/tagged tests, both vet modes, full race, maturity/import checks, and `git diff --check` pass.
- Final `FuzzControlStringFraming` and `FuzzParserAdvanceDoesNotPanic` runs each completed for at least 60 seconds. Performance captures were then run as a separate isolated step so sustained 32-worker fuzz heat did not distort the single-P laptop baseline.
- During Slice 13.2, four isolated captures on byte-identical VT/core sources could not reproduce the original discard median and consistently exceeded the strict 3% noise band. The baseline was recalibrated without source changes; its immediate independent capture passed at -0.60% discard and +1.32% overflow with zero allocations.

## Required later-slice gate

Every later parser/Kitty slice captures the `control` suite and compares it against this raw artifact. Matching medians may regress by at most 3%; any worst-sample `B/op` or `allocs/op` increase fails.
