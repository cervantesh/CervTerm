# Architecture Maturity Slice 6.2b — Mux Protocol-Scheduling Coordinator

Date: 2026-07-24
Execution predecessor: Slice 6.2a, merged by `801285e` (PR #218)
Branch: `arch/l3-01b-mux-protocol-scheduling`
Finding state: L3-01 remains **partial**; L3-09 remains open to Slice 4.8; 6.2d is deferred.

## Scope

Slice 6.2b delegates unchanged Kitty/Sixel/iTerm dispatch and image expiry/completion application beneath five existing private `Mux` shims. The private generic `protocolSchedulingController` is import-free and zero-field. `Mux` remains authoritative for scheduler, queue and pending-map ownership, panes and stores, clocks, erased result routing, replies, diagnostics, events, topology and lifecycle. Operation-scoped adapters are passed by value; no controller retains `Mux`, panes, scheduler/store state or mutable state, and no exported protocol-scheduling bypass is introduced.

The two exact `KnownDefect_L3_09_*` characterization tests continue to expire in Slice 4.8. The only Slice 6.2b facade expiry is `TODO(L3-01; expires Slice 6.2d)`.

## Relations and disposition

- `execution_predecessor`: Slice 6.2a / PR #218 / merge `801285e`.
- `semantic_depends_on`: none for preparatory parity delegation; L3-02/L3-03/L3-04/L3-08/L3-09/L3-10 plus 6.2a-c before formal closure.
- Disposition: Slice 6.2b is **complete only as preparatory work**. L3-01 remains **partial**. L3-09 remains open to Slice 4.8. Phase 6 and 6.2d remain open; 6.2d is deferred.

## Exact changed-path allowlist

No generated exceptions. The immutable T-through-G slice is restricted to:

```text
docs/architecture-maturity/implementation-plan.md
docs/architecture.md
docs/validation/architecture-maturity-slice-6.2b.md
docs/validation/architecture-maturity-slice-6.2b/benchmarks-base.txt
docs/validation/architecture-maturity-slice-6.2b/benchmarks-candidate.txt
docs/validation/architecture-maturity-slice-6.2b/gates.txt
docs/validation/architecture-maturity-slice-6.2b/scope-and-commits.txt
internal/mux/mux.go
internal/mux/mux_iterm.go
internal/mux/mux_kitty.go
internal/mux/mux_protocol_scheduling_test.go
internal/mux/mux_sixel.go
internal/mux/protocol_scheduling_controller.go
internal/mux/protocol_scheduling_controller_test.go
scripts/check-maturity-gates.go
```

## Atomic commits

| Class | Commit | Purpose |
|---|---|---|
| T | `ac708ca` | `test(mux): characterize protocol scheduling parity`; pin ordering, retained defects, idle/completion/allocation behavior. |
| A | `fb30dff` | `refactor(mux): add protocol scheduling controller seam`; add private unwired seam. |
| M | `64d407e` | `refactor(mux): split protocol scheduling adapters`; mechanically split operation adapters. |
| W | `4ba1b3e` | `refactor(mux): wire protocol scheduling controller`; wire unchanged private shims. |
| G | pending | `refactor(mux): guard protocol scheduling controller delegation`; guards, benchmarks, evidence and minimal docs. |

The maturity gate fixes exact T/A/M/W identities, subjects and direct ancestry. Before G, `HEAD` must equal immutable W and scope is the union of `801285e..4ba1b3e` plus tracked, staged and nonignored untracked paths. Once exact-subject G exists on the active branch, `HEAD` must equal G and the nonignored worktree must be clean. Merge/main history validates only the immutable `801285e..G` range; later unrelated commits are outside Slice 6.2b. A history-limited shallow checkout validates this documented sequence and allowlist, uses available commit metadata to require identifiable G's exact sole parent to be W, fails the active slice safely when parentage cannot be proved, and otherwise requires an unidentified pre-G active slice to remain exactly at W. Later-main shallow state remains allowed.

### A-to-W seam refinement

A introduced one combined `dispatch` controller method as an unwired seam. W deliberately refined that seam into independent `dispatchKitty`, `dispatchSixel` and `dispatchITerm` methods before wiring because production already has real independent call sites: parser callbacks dispatch one selected protocol, image expiry dispatches each protocol independently, and the `advancePane` and EOF paths impose the existing Kitty→Sixel→iTerm order. M supplies the matching operation adapter methods; W does not rewrite A history. `TestMuxProtocolSchedulingIndependentSingleProtocolCalls`, the advance-path ordering test, and the focused EOF mixed-protocol ordering test preserve those independent-call and ordering behaviors without a production behavior change.

## Static and runtime guards

- The controller remains a private, import-free, zero-field generic struct with exactly two parameters constrained by the exact two private ports.
- Port budget is exactly 5: dispatch has exactly three methods and apply has exactly two; all five names and full signatures are fixed and forbidden owner/state types are rejected.
- The controller has exactly five production methods, exact receivers/signatures and one-call bodies; its constructor has the exact zero-value body. Any other production method, controller alias, controller/adapter hidden recursively in pointers, slices, arrays, maps, named or anonymous structs, aliases or fields, retained adapter field, exported seam or bypass is rejected.
- `Mux` contains exactly one exact controller field and constructor initializer. The only controller calls are the exact five one-call private shims; the matching five controller-to-port calls are the only other reserved selector calls. The `advancePane` and EOF callers are fixed to exact Kitty→Sixel→iTerm order.
- Exactly one Slice 6.2b `TODO(L3-01; expires Slice 6.2d)` remains. Exactly two named `KnownDefect_L3_09_*` tests remain and both expire in Slice 4.8.
- Synthetic guard self-tests reject controller aliases, recursively nested alias/field containers, alternate controller fields, direct-adapter bypasses, missing shallow contracts/allowlist entries, wrong-parent G metadata, active post-G descendants and dirty post-G worktrees while allowing later-main shallow state.

## Same-host performance evidence

Windows/amd64, AMD Ryzen 9 7940HX, Go 1.25.8. The base is `fb30dff`; the candidate is immutable W `4ba1b3e` plus current uncommitted G guard/test/docs source. The canonical capture contains exactly ten interleaved 2-second, `-cpu=1`, `-count=1`, `-benchmem` samples per identity and benchmark, alternating base then candidate for rounds 01–10 after an unrecorded one-second warm-up. The temporary base overlay only renames the old Drain benchmark to `BenchmarkMuxProtocolDrainIdle` and adds the retained true-dispatch benchmark body that directly calls the three current process shims on empty queues. No production source is overlaid.

Raw samples and the reproducible overlay procedure are in `architecture-maturity-slice-6.2b/benchmarks-{base,candidate}.txt`. Canonical ten-sample medians are:

| Benchmark | Base | Candidate | Delta | Allocation disposition |
|---|---:|---:|---:|---|
| True protocol dispatch idle | 9.373 ns/op | 12.710 ns/op | +3.337 ns, +35.60% | 0 B/op, 0 allocs/op on both |
| Completion discard | 12.200 ns/op | 20.600 ns/op | +8.400 ns, +68.85% | 0 B/op, 0 allocs/op on both |
| Protocol Drain/expiry idle | 114.600 ns/op | 122.850 ns/op | +8.250 ns, +7.20% | 0 B/op, 0 allocs/op on both |
| All-disabled Drain + deadline idle | 28.420 ns/op | 31.300 ns/op | +2.880 ns, +10.13% | 0 B/op, 0 allocs/op on both |
| Ingress ASCII | 11,267.5 ns/op | 11,263.5 ns/op | -4.0 ns, -0.04% | 1,016 B/op and 3 allocs/op on both |
| Work scheduler | 302.550 ns/op | 290.500 ns/op | -12.050 ns, -3.98% | 0 B/op, 0 allocs/op on both |

The four protocol percentages above 3% are not represented as threshold passes. Retained candidate-source attribution uses ten 2-second samples and no benchmark-specific production branch: true dispatch direct adapters 10.525 → controller shims 13.615 ns (+3.090 ns controller), completion direct adapter 15.940 → controller 21.940 ns (+6.000 ns controller), enabled expiry direct adapter 65.945 → controller 67.850 ns (+1.905 ns), and all-disabled nil-path expiry direct adapter 4.729 → controller 5.786 ns (+1.057 ns). Base-to-candidate direct-stage comparisons add +1.152 ns for dispatch and +3.740 ns for completion; staged totals are within 0.905 ns and 1.340 ns respectively of the independently interleaved overall deltas. Completion’s accepted structural overhead is therefore stated as +8.400 ns overall, never hidden behind its percentage.

The canonical Drain benchmark is now named and described as Drain/expiry, not dispatch. Its production scaffolding is unchanged across identities; the only hot-path source change is expiry extraction/delegation. The +8.250 ns full-context delta is reported alongside, not replaced by, the isolated +1.905 ns direct-to-controller measurement. Likewise the all-disabled +2.880 ns full-context delta is reported alongside the matched +1.057 ns nil-path measurement. Compiler diagnostics (`-gcflags='-m=2'`) confirm that Drain itself is not inlineable while the shim/controller path inlines to a generic-dictionary indirect adapter call, so the full-context composition is mapped to the same compiler-visible expiry structure and is not claimed as dispatch cost. Ingress is flat, work scheduler improves, and no allocation increased. No unexplained regression outside the exact dispatch/completion/expiry structural changes exceeds 3%.

## Verification disposition

Exact commands and results are recorded in `architecture-maturity-slice-6.2b/gates.txt`; immutable scope and commit evidence are in `scope-and-commits.txt`. Windows real-GUI qualification is `UNRUN`: this preparatory slice does not change window/resource ownership, renderer behavior, geometry, pixels, feature defaults or support claims. No release, changelog, deployment or commit is produced.
