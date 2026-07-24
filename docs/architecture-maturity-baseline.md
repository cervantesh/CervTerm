# Architecture Maturity Phase 0 Baseline

Captured: 2026-07-23
Production commit: `e9f9b2c0666f1392c8a1b74ca6b6cb0be261b000` (`v0.10.0-beta.1`)
Host: Windows/amd64, Go 1.25.8, AMD Ryzen 9 7940HX

The architecture files were dirty while commands ran, but no production Go file differed from the named commit. Raw bytes are committed under `docs/validation/architecture-maturity-phase0/`; `.gitattributes` fixes LF and `sha256sum <path>` reproduces each hash. Regenerate the complete set with `go run ./scripts/capture-architecture-maturity-baseline.go -count 10`.

## Common gate

Passed:

```text
go run ./scripts/check-maturity-gates.go
go test ./...
go vet -unsafeptr=false ./...
go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1
go run ./scripts/check-phase15-recovery.go -race
```

Raw capture: `docs/validation/architecture-maturity-phase0/common-gate.txt`; SHA-256 `66dbd05b293a4cdc688f021c779dcf5d8d564aab8933a8cf68b5f2031fce3aef`.

The maturity gate reports the known `internal/fontglyph/color_colr_render.go` and `internal/fontglyph/backend.go` large-file exceptions; both are explicit extraction targets, not new regressions.

## Production package graph

Method: the checked-in capture script parses imports from every file in the canonical scored-file manifest, irrespective of GOOS/build tags, maps directories to module package paths, sorts/deduplicates all local edges, and applies Tarjan SCC detection.

- Packages: **37**
- Directed all-production-file edges: **71**
- Cycles: **0**
- Raw graph: `docs/validation/architecture-maturity-phase0/package-graph.txt`; canonical LF SHA-256 `2f9c946eebe0b8e92cd77f3d90e693e9af9be8c2e1b6a3dcc4e25a53eb57eeb8`

Every structural slice must preserve zero cycles across all tracked production files, not only the host's selected build. ADR-0021 additionally forbids public font subsystem packages from importing the root facade or one another while allowing the exact private-face/stable-leaf edges.

## Ten-sample performance baseline

Command: `go run ./scripts/capture-parity-baseline.go -count 10`.
Raw report: `docs/validation/architecture-maturity-phase0/performance.txt`; SHA-256 `faaae0ed88046564d048dd385387671230e39083618377fa7b9cddb7e39b6626`.

| Benchmark | Median ns/op | Min | Max | Allocation invariant |
|---|---:|---:|---:|---|
| VT parser throughput | 3029.0 | 2590 | 3126 | 0 B, 0 allocs |
| Core reuse | 2914.5 | 2801 | 3062 | 0 B, 0 allocs |
| New terminal | 42894.5 | 40008 | 43690 | 158128 B, 4 allocs |
| Render snapshot reuse | 9236.0 | 7868 | 9471 | 2 B, 0 allocs |

Acceptance follows the inherited Phase 15 rule: allocation regressions block; an unexplained median regression greater than 3% blocks. Compare on the same class of machine/toolchain, otherwise record the environment difference and establish a reviewed equivalent baseline before claiming parity.

## Multi-window geometry/lifecycle, accessibility and public-output evidence

The following focused tests passed:

- stable-origin cross-window actions;
- per-pane-metric cross-window pane movement and stale-source atomicity;
- exact-window close while siblings continue framing;
- independent window bundle lifecycle;
- runtime native/mux publish-activate-close ordering;
- complete `internal/accessibility` suite;
- VT/mux public-output and projection tests.

Raw focused capture: `docs/validation/architecture-maturity-phase0/focused-evidence.txt`; SHA-256 `cd56b91138781ab426019af8035b2515c1eaaa87aaba6f44db8666f5c572d2ae`. This is intentionally not called a shared-config trace: baseline `e9f9b2c` has per-App configuration state and no process-owned all-window config generation. L1-02/ADR-0018 owns that correction and its two-window prepare/commit/rollback tests.

## Known baseline defects

Characterization may preserve an accepted defect only under a `KnownDefect_<finding-id>_*` name and explicit expiry slice. It is not desired behavior. The correction slice must replace it with the corrected invariant before that finding can close.

## Scoring baseline

The reconciled starting scores are Architecture maturity 7.4, Clean Code 6.0, GRASP 6.4, Dependency graph hygiene 8.1, Domain isolation 7.0, Ownership/transactions 7.5, Test/guardrail maturity 8.8, Overall 7.3. They are historical context, not automatic credit. Final acceptance follows `architecture-scoring-protocol.md` and requires the lower final score from both teams to be at least 8.0 for every row.
