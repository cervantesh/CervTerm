# Phase 13 bounded image-store baseline

Date: 2026-07-21
Slice: 13.2
Base revision: `fb97fd6`

## Scope

This first-result baseline covers the inert, protocol-neutral `internal/termimage` foundations. No production runtime constructs a process budget or pane store in this slice.

Hard caps follow ADR 0014: 8/32 pending transfers, 8/32 MiB encoded bytes, 64/256 MiB decoded residency, 256/1,024 images, and 1,024/4,096 placements for pane/process. Operational pane limits may only lower the encoded, decoded, image, and placement ceilings.

## Reproducible capture

```bash
go run ./scripts/capture-phase13-benchmark.go -suite store -out docs/validation/phase-13-image-store-baseline.txt
go run ./scripts/capture-phase13-benchmark.go -suite store -out phase13-store-candidate.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-image-store-baseline.txt phase13-store-candidate.txt
```

The capture uses the standard warmed, single-P, 2-second, ten-sample method and records separate benchmark-harness and measured-source digests.

| Benchmark | Median ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkProcessBudgetReserveRelease` | 44.70 | 64 | 1 |
| `BenchmarkStoreBeginTransferCancel` | 236.25 | 336 | 4 |
| `BenchmarkStoreAcquireMiss` | 1.73 | 0 | 0 |

The immediate independent candidate passed the 3% median and worst-allocation gates. Acquire misses remain allocation-free. Reservation/transfer allocations are enabled-only capability objects and become regression ceilings for later store slices.

## Qualification notes

- Independent review found and closed unbounded closed-transfer map retention, passive-only expiry, missing decoded-candidate construction, and premature generation mutation.
- Transfer timers are bounded by the 8/32 pending-transfer caps, release leases autonomously at ten seconds, and remove only their exact pending-map entry.
- Decoded candidates reserve image/decoded residency before pixel allocation, bind an atomic store epoch, survive reset only as invalid worker-owned leases, and release exactly once on stale worker return.
- Full untagged/tagged tests, both vet modes, full race, maturity/import checks, and `git diff --check` pass.
- `FuzzStoreLifecycle` and `FuzzCheckedRGBABytes` each completed for at least 60 seconds.
- Repeated source-identical captures on this laptop showed timing variance above 3% despite identical measured-source digests. Source-identical timing is therefore diagnostic; worst-allocation gates remain mandatory, and any measured-source change restores the hard 3% timing gate.
