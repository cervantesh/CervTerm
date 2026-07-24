# Architecture Maturity Slice 6.2a — Mux Session-Ingress Coordinator

Date: 2026-07-24
Execution predecessor: Slice 6.3c, merged by `c2a0137` (PR #217)
Branch: `arch/l3-01a-mux-session-ingress`
Finding state: L3-01 remains **partial**; formal closure is deferred to Slice 6.2d after L3-02/L3-03/L3-04/L3-08/L3-09/L3-10, preparatory 6.2a-c and the 6.2d execution predecessor.

## Scope

Slice 6.2a delegates accepted-record validation and data-before-end ordering beneath the unchanged `Mux.Drain` entry point. The private generic `sessionIngressController` is zero-field. `Mux` remains authoritative for ingress scheduling, session registry and pane ownership, parser/terminal mutation, protocol and image outcomes, event addressing, topology and lifecycle. Operation-scoped adapters are created inside `Drain`; no controller retains `Mux`, registry, pane or mutable state, and no exported ingress bypass is introduced.

Tagged L3-02, L3-04 and L3-08 known defects remain assigned to their documented expiry slices. The only new expiry is `TODO(L3-01; expires Slice 6.2d)` on the preparatory facade.

## Relations and disposition

- `execution_predecessor`: Slice 6.3c / PR #217 / merge `c2a0137`.
- `semantic_depends_on`: none for preparatory parity delegation; L3-02/L3-03/L3-04/L3-08/L3-09/L3-10 plus 6.2a-c before formal closure.
- Disposition: Slice 6.2a is **complete only as preparatory work**. L3-01 and Phase 6 remain open; 6.2d is deferred.

## Exact changed-path allowlist

No generated exceptions. The immutable T-through-G slice is restricted to:

```text
docs/architecture-maturity/implementation-plan.md
docs/architecture.md
docs/validation/architecture-maturity-slice-6.2a.md
docs/validation/architecture-maturity-slice-6.2a/benchmarks-base.txt
docs/validation/architecture-maturity-slice-6.2a/benchmarks-candidate.txt
docs/validation/architecture-maturity-slice-6.2a/gates.txt
docs/validation/architecture-maturity-slice-6.2a/scope-and-commits.txt
internal/mux/mux.go
internal/mux/mux_kitty_test.go
internal/mux/mux_session_ingress_test.go
internal/mux/session_ingress_controller.go
internal/mux/session_ingress_controller_test.go
internal/mux/session_registry.go
scripts/check-maturity-gates.go
```

## Atomic commits

| Class | Commit | Purpose |
|---|---|---|
| T | `271325b` | `test(mux): characterize session ingress ordering`; pin parity and retained defects. |
| A | `45514c8` | `refactor(mux): add session ingress controller seam`; add private unwired seam. |
| M | `66072c9` | `refactor(mux): split session ingress adapters`; mechanically split adapters. |
| W | `aa1bfd4` | `refactor(mux): wire session ingress controller`; wire the unchanged public facade. |
| G | pending | `refactor(mux): guard session ingress controller delegation`; guards, benchmarks, evidence and minimal docs. |

The maturity gate fixes exact T/A/M/W identities, subjects and direct ancestry. Before G, `HEAD` must equal immutable W and scope is the union of `c2a0137..aa1bfd4` plus tracked, staged and nonignored untracked paths. Once exact-subject G exists on the active branch, `HEAD` must equal G and the nonignored worktree must be clean. Merge/main history validates only the immutable `c2a0137..G` range; later unrelated commits are outside Slice 6.2a. A history-limited shallow checkout still determines active branch, HEAD and worktree state, validates this documented sequence and current production structure, enforces exact G/clean state when G is identifiable, and otherwise requires an active slice branch to remain at W. Later main history is not rejected. Synthetic self-tests cover the combined shallow/post-G policy, active post-G/dirty rejection, missing shallow contract/allowlist entries, and route-exclusivity bypasses through local aliases, transitive global aliases, and controllers stored under alternate struct fields.

## Static and runtime guards

- The controller remains a private, import-free, zero-field generic struct with exact `ownerPort sessionIngressOwnerPort` and `applyPort sessionIngressApplyPort` parameters and constraints.
- Private port interface names, method names and full signatures are exact: `acceptSessionIngress() bool`, `applySessionIngressData([]Event, []byte) []Event`, and `applySessionIngressEnd([]Event, error) []Event`; aggregate budget 3, largest port 2 methods, hard maximum 5.
- The only production controller method is exact-signature `route(events []Event, owner ownerPort, apply applyPort, data []byte, end error) []Event`; AST source order is pinned to accept → data → end in addition to runtime traces.
- Port types reject `Mux`, registry, pane, maps, callbacks/function bags, channels, `any` and empty-interface state.
- Production AST scanning excludes tests and reserves the exact private selector method name `route` throughout `internal/mux` through Slice 6.2d. Every production selector call named `route` is relevant regardless of inferred receiver identity; the only permitted call is the single exact `m.sessionIngress.route(events, accepted, operation, record.data, record.err)` inside `(*Mux).Drain` in `mux.go`. Exported ingress declarations, aliases, alternate controller fields/methods, and route/adaptation bypasses are rejected.
- Exactly one `TODO(L3-01; expires Slice 6.2d)` remains on the preparatory facade.
- Existing characterization pins stale-owner rejection, parser/reply/public-output ordering, metadata, EOF/error ordering, callback revalidation, allocation parity and retained known defects. A deterministic image-disabled test also pins that `ClosePane` causes already queued ingress for the detached pane to be consumed and dropped without events; no nondeterministic image-vs-ingress select is tested.

## Same-host performance evidence

Windows/amd64, AMD Ryzen 9 7940HX, Go 1.25.8. `origin/main` was `c2a0137`; candidate was immutable W `aa1bfd4` plus the uncommitted G guard/evidence source. Base-first and candidate-first orders each ran seven `-benchtime=500ms -cpu=1` samples; that valid base raw capture is retained. The base used a temporary benchmark-only overlay for the two original ingress benchmarks. After adversarial repair, the final candidate suite and a focused attribution confirmation each ran seven samples. `BenchmarkMuxSessionIngressStaleAcceptancePaths` now lives in `internal/mux/mux_session_ingress_test.go`, so it will travel with G rather than depending on a temporary overlay.

| Benchmark | Base median (14) | Candidate median (14) | Delta | Allocation disposition |
|---|---:|---:|---:|---|
| Rejected stale ingress | 48.485 ns/op | 58.680 ns/op | +10.195 ns, +21.03% | 0 B/op, 0 allocs/op on both |
| ASCII ingress | 9,345.5 ns/op | 9,288.0 ns/op | -57.5 ns, -0.62% | 3 allocs/op on both; median 1,089 → 1,086 B/op |
| Existing all-disabled image idle | 27.420 ns/op | 27.705 ns/op | +0.285 ns, +1.04% | 0 B/op, 0 allocs/op on both |

The original rejected-stale comparison is +10.195 ns (+21.03%), above 3% but bounded in absolute time. The retained `BenchmarkMuxSessionIngressStaleAcceptancePaths` makes that attribution reproducible from source using the production `Drain` prelude. Focused seven-sample medians are 48.81 ns/op for stale inline acceptance, 55.35 ns/op for operation-adapter acceptance (+6.54 ns), and 58.61 ns/op for active generic-controller routing (+3.26 ns). The staged +9.80 ns is within 0.395 ns of the original cross-commit +10.195 ns; the separate full-suite run is retained to expose run-to-run noise. Every attribution path is 0 B/op and 0 allocs/op. Fresh full-suite medians are 59.14 ns/op for production rejected stale, 9,422 ns/op and 3 allocations for ASCII, and 27.78 ns/op with zero allocations for all-disabled image idle. No unexplained allocation increase or material hot-path regression remains.

Raw captures are in `docs/validation/architecture-maturity-slice-6.2a/`.

## Verification disposition

Exact commands and results, including the seven-sample retained-source candidate rerun, are recorded in `architecture-maturity-slice-6.2a/gates.txt`; raw benchmark samples are in `architecture-maturity-slice-6.2a/benchmarks-{base,candidate}.txt`; immutable scope and commit evidence are in `scope-and-commits.txt`. Real-GUI qualification is `UNRUN`: this preparatory slice does not change window/resource ownership, renderer behavior, geometry, pixels, feature defaults or support claims. No release, changelog, deployment or commit was produced.
