# Phase 15 performance qualification

Date: 2026-07-23
Candidate: `fd98c7eebe19b31c270996889ba95f81584ef16d`
Host: Windows amd64, AMD Ryzen 9 7940HX, Go 1.25.8, `GOMAXPROCS=1`
Result: **PASS with explicit waiver P15-W01**

Machine-readable evidence: [`phase-15-performance.json`](phase-15-performance.json).

## Reproducible command

```text
go run ./scripts/capture-phase15-benchmarks.go -waiver P15-W01
```

The command requires a clean tree, a new child directory under `dist/`, the pinned benchmark harness from `0673a5aa2d127080087a38446c3a6a16664562a9`, and the recorded toolchain/CPU environment. It checks source digests, rejects non-finite benchmark values, re-runs historical production commits in detached worktrees, alternates baseline and candidate samples to reduce thermal/order bias, and verifies that `HEAD` and the worktree remain unchanged.

## Comparison protocol

- Phase 0 (`7d64cc9`): identical pinned harness, three samples, 2-second benchtime, 15% ceiling.
- Phase 13/14 inherited paths (`b1eec133`): identical pinned harness, ten interleaved samples, 2-second benchtime, 3% ceiling.
- Candidate integration paths: ten samples, fixed execution-time, byte, and allocation budgets.
- Any allocation-count increase fails. Candidate-only budgets cover terminal startup memory, input encoding, resize/reflow, font environment rebuild, 32-tab/8-window projection, semantic projection, disabled image idle, accessibility, shared scheduling/store operations, Sixel, and iTerm2 image paths.

## Results

All unwaived inherited rows passed:

- text-only parser: `+0.86%`;
- text-only core reuse: `-0.36%`;
- text-only snapshot: `-0.58%`;
- control discard: `-3.16%`;
- process budget: `+2.40%`;
- store miss: `+1.07%`;
- store transfer/cancel: `-1.63%`;
- disabled draw: `-0.53%`;
- disabled frame: `-0.19%`.

Candidate-only budgets all passed. Notable medians were 27.9 ns/op with zero allocations for disabled image idle, 5.1 µs/op with zero allocations for semantic projection, 5.48 ms/op for the bounded 120x32/80x40 reflow case, and 11.4 µs/op / 125,344 B/op / 4 allocs/op for terminal startup state.

## P15-W01

P15-W01 was explicitly supplied on the command line and is limited to three measured consequences of the accepted Phase 10-14 metadata/protocol architecture:

1. `BenchmarkCaptureReuse`: `+309.70%`, ceiling raised to 400%. Detached snapshots now project semantic, image, accessibility, and lifecycle metadata. Steady-state allocation count remains zero; the amortized setup row reports 1 B/op.
2. `BenchmarkCoreReuseVsNew/new-terminal`: terminal creation is 66.14% faster, retains 4 allocations, and grows from 156,208 to 158,128 B/op (`+1.23%`) for bounded metadata state.
3. `BenchmarkPhase13ControlStringOverflow`: `+8.07%`, ceiling raised to 15%. Selected-DCS preamble classification and bounded adapter routing are now performed; streaming remains allocation-free.

The waiver does not relax disabled draw/frame, ordinary text, store, accessibility, scheduler, image decoder, or candidate integration budgets. An unknown or inapplicable waiver fails the harness.

## Remaining process qualification

This slice qualifies deterministic code paths and startup-state allocation. Packaged GUI cold-start latency, steady-state RSS, and idle process CPU remain part of the Windows/macOS/Linux platform matrix and must be closed before P15-02 is marked complete.
