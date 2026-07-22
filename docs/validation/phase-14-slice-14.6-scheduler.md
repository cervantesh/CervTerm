# Phase 14 Slice 14.6 — Shared Decode Scheduler Validation

Date: 2026-07-22
Candidate: `f88ed06` (pre-amend implementation commit)
Parent: `0fb9a0f`

## Correctness gates

Passed:

- `go test ./... -count=1`
- `go test -tags glfw ./... -count=1`
- `go vet -unsafeptr=false ./...`
- `go vet -unsafeptr=false -tags glfw ./...`
- `go test -race ./... -count=1`
- `go test -race -tags glfw ./internal/frontend/glfwgl ./internal/frontend/gpu ./internal/workscheduler ./internal/kitty ./internal/mux -count=1`
- maturity, Phase 13 import, Phase 14 import, and `git diff --check` gates

Independent scheduler/concurrency review found no remaining blocker after fixes for atomic completion publication, bounded result backpressure, shutdown wakeup, type-erased ownership, nil results, queue-inclusive deadlines, and late-candidate rejection.

## Scheduler benchmark

`BenchmarkSchedulerSubmitComplete` ran ten times with `-benchtime=2s -cpu=1`:

- median: approximately 315 ns/op
- allocations: 0 B/op, 0 allocs/op in every run

## Disabled text-only control

The parent and candidate use the same text benchmark harness hash (`7997496ee147c253b4a38e9c5f2d439cede13eca619fdeb3ce42c0ceed5d215a`) and byte-identical measured text-path sources. All measurements remained 0 B/op and 0 allocs/op.

Two interleaved parent/candidate pairs showed non-repeatable host-frequency noise rather than a code-correlated regression:

| Pair | Snapshot | Core reuse | Parser |
|---|---:|---:|---:|
| 1 | -2.85% | -4.16% | +7.73% |
| 2 | -4.48% | +6.54% | -2.51% |

The regressing benchmark changed between pairs while the measured sources were identical. Independent performance adjudication accepted the evidence: no unexplained implementation regression and no allocation/wake regression on the disabled path.
