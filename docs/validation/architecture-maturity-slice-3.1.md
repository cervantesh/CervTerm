# Architecture Maturity Slice 3.1 — Executable Owner Capability

Date: 2026-07-29
Branch: `arch/l3-02-owner-thread-capability-rebuild`
Baseline: `9fe0bd0287ed402fe90b58a5fba7d6413897c1af`
ADR: ADR-0017 accepted.

L3-02 is **closed only when** the exact history, structural guards, zero-mutation rejection tests, physical performance evidence, full validation family, CI, and clean merged-state checks pass together.

## Ownership model

The shared `internal/ownerthread` leaf supplies exact OS-thread identity. `mux.Owner` and `termimage.StoreOwner` pin the creating goroutine, capture the native identity, and release it exactly once after committed shutdown. The GLFW root captures that same kind of identity plus a monotonic loop epoch before attaching a projection. Every root native path checks the live thread and epoch before use.

Process and window mutations use ephemeral process and window mutation scopes carrying owner generation, exact native thread, and a one-dispatch nonce. `WindowOwner` additionally binds a non-reusable `WindowIdentity{ID, Incarnation}` and detached current-context attestation. Private mux sinks revalidate the scope and live pane/tab/window origin before their first mutation.

The primary App alone holds the concrete process host. Child projections receive a narrow `windowMuxCapability` and typed message router; process lifecycle messages carry the exact origin identity and fail closed after teardown or incarnation change. The router does not return the controller, process service storage, `Owner`, or `Mux`.

`PreparedStoreState` and `PreparedStoreClose` bind exact owner identity and generation. Commit, Abort, and Close reacquire fresh prepared scopes. Close preparation uses an atomic reservation—not a retained mutation nonce—so competitors receive `ErrOwnerBusy` while wrong-thread, stale, or failed completion leaves ownership and store state intact.

`Store` serializes only owner claim versus detached reset/close admission; normal owner mutations remain scope-gated and resource reads load one immutable atomic snapshot. Published transitions remain lifecycle-accounted until Commit/Abort, so close/reset cannot bypass retired leases or reservations. `PreparedImageStoreClose` copies share one resolution state, validate the native owner before reading terminal fields, and use an atomic terminal publication generation for empty-store preflights.

Unpublished tab/restore failures retain at most one mux-owned rollback candidate until every pane has reacquired close authority and detached exact registry ownership. Cleanup runs in reverse order, joins spawn/session/close errors, launches no reader before publication, emits no interim events, and blocks new publication until retry completes.

## Exact history

| Class | Commit | Parent | Subject |
|---|---|---|---|
| T | `4a9c2800ac2bbd943eb93e892f45abe01a4328bd` | `9fe0bd0287ed402fe90b58a5fba7d6413897c1af` | `test(mux): characterize owner mutation boundary` |
| A | `cf834b7870e34689c789f38aeaf6de4914b34675` | T | `refactor(mux): add owner capability seam` |
| W | `27070f32395be7803b6fc11388a08a5c32d3d3a1` | A | `refactor(mux): wire executable owner capability` |
| G | G is derived as the unique full-history child of W | W | `refactor(mux): guard executable owner capability` |

The guard rejects every shallow checkout; full-history validation is mandatory. It pins T/A/W exactly, derives G by its unique exact subject and sole parent, and verifies a sorted 158-path two-dot allowlist. W is rejected as a benchmark baseline because it contains the production change; the immutable pre-slice baseline is current `main` at `9fe0bd0`.

## Structural and rejection guards

Recursive AST and go/types guards cover direct writes, atomics, aliases, method values, helpers, closures, generics, containers, and capability locators across core, mux, termimage, frontend, and ownerthread. They require:
Windows runs the complete typed GLFW/frontend closure. Headless Unix runs the same recursive frontend AST closure plus typed mux/core/termimage/ownerthread analysis, avoiding a false dependency on unavailable X11/OpenGL development packages; the mandatory Windows CI lane supplies the complementary typed frontend proof.

- every public Owner/WindowOwner mutation to enter, check, defer leave, and pass its exact scope to a private sink;
- exact process/window capability inventories with no callable concrete service locator;
- exact thread, loop-epoch, window incarnation, context, owner, nonce, and origin checks;
- zero-mutation fingerprints for wrong-thread, wrong-owner/origin/incarnation, stale, busy, closed, and expired requests;
- fresh prepared scopes plus atomic close reservation;
- preservation of Slice 6.2a/b/c delegation, budgets, ordering, and retained defect pins;
- preservation of hidden initial-window creation, two normal accounted presentations, compositor flush/reveal, and focus ordering.
- an automated detached-worktree rollback rehearsal that reverts G, W, then A and proves the resulting tree is exactly retained T.

## Physical performance

`capture-slice31-evidence.go` built baseline `9fe0bd0` and production candidate W in detached full-history worktrees. The candidate test binary overlays only the final hashed characterization harness. Six workloads each received ten physical samples per side in ABBAx5 order: 120 timed processes after 12 warmups, at two seconds per process.
`StartupProxy` measures the underlying mux startup allocation path; one-time `Owner`/`WindowOwner` capability construction is validated by lifecycle/race tests rather than the no-allocation mutation fast-path gate.

| Workload | Baseline median ns/op | Candidate median ns/op | Delta | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|
| ProcessMutation | 57,682.5 | 55,310.5 | -4.112166% | 0 → 0 | 0 → 0 |
| WindowMutation | 7,736.5 | 5,194.0 | -32.863698% | 560 → 560 | 4 → 4 |
| PaneMutation | 6,514.0 | 4,624.0 | -29.014430% | 513 → 508 | 3 → 3 |
| ImageMutation | 24,484.0 | 9,568.5 | -60.919376% | 27,832 → 14,240 | 18 → 15 |
| StartupProxy | 2,167.5 | 1,807.0 | -16.632065% | 3,632 → 3,024 | 28 → 28 |
| HeadlessProxy | 51,590.5 | 52,694.0 | +2.138960% | 98,400 → 98,400 | 3 → 3 |

All medians are within the `<=3.000000%` regression ceiling and no workload increases bytes/op or allocations/op. Candidate production source SHA-256 is `760333f3c82c9a06971cd1d0132ea5bb0118eb3beac1a75f9a5a0fe2a341792c`; baseline/candidate test binaries are `334a3ef9a3b83b278f7d2bfc9a95b5fd907b3a520e22caf1d5e5d1627ec8a9d7` / `0a095f1c966680aa85bb80ccd96732e162a1d120c7e2f83b089c264f920e2ba1`.

## Closure

Automated default, GLFW, race, vet, maturity, recovery, fuzz, cross-target compile, exact-history, and performance gates are recorded under `docs/validation/architecture-maturity-slice-3.1/`. PR #229 passed Windows CI, Linux headless CI, CodeQL, and merged-state verification, and merged as `7d753ee1513531d99cf47a9df420eea136222110`; L3-02 is closed. Interactive Windows GUI qualification remains separately recorded as `UNRUN`; it does not convert the automated ownership claim into a broad platform claim.
