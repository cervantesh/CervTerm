# Architecture Maturity Slice 6.2c — Mux Restore Coordinator

Date: 2026-07-24
Execution predecessor: Slice 6.2b, merged by `36450fe` (PR #219)
Branch: `arch/l3-01c-mux-restore-coordinator`
Finding state: L3-01 remains **partial**; L3-02/L3-04/L3-07/L3-09/L3-10 remain open; 6.2d is deferred.

## Scope

Slice 6.2c delegates unchanged fresh-session snapshot projection and restore preparation/publication/abort forwarding beneath the five existing public `Mux` facades. The private generic `restoreCoordinator` is import-free and zero-field. `Mux` remains authoritative for model and identity allocation, registry/pane/PTY/parser/terminal/image ownership, pending candidates, publication, reader launch, reverse-order abort, topology and lifecycle. Operation-scoped adapters are passed by value; no controller retains `Mux`, model, registry, pane or mutable state, and no exported restore bypass is introduced.

The exact `KnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread` and `KnownDefect_L3_07_FreshSessionUsesObservedTerminalCWD` characterizations remain assigned to Slices 3.1 and 4.3. The only Slice 6.2c facade expiry is `TODO(L3-01; expires Slice 6.2d)`.

## Relations and disposition

- `execution_predecessor`: Slice 6.2b / PR #219 / merge `36450fe`.
- `semantic_depends_on`: none for preparatory parity delegation; L3-02/L3-04/L3-07/L3-09/L3-10 remain open before formal closure.
- Disposition: Slice 6.2c is **complete only as preparatory work**. L3-01 remains **partial**. Phase 6 and 6.2d remain open; 6.2d is deferred.

## Exact changed-path allowlist

No generated exceptions. The immutable T-through-G slice is restricted to:

```text
docs/architecture-maturity/implementation-plan.md
docs/architecture.md
docs/validation/architecture-maturity-slice-6.2c.md
docs/validation/architecture-maturity-slice-6.2c/benchmarks-base.txt
docs/validation/architecture-maturity-slice-6.2c/benchmarks-candidate.txt
docs/validation/architecture-maturity-slice-6.2c/gates.txt
docs/validation/architecture-maturity-slice-6.2c/scope-and-commits.txt
internal/mux/fresh_session.go
internal/mux/mux.go
internal/mux/mux_restore.go
internal/mux/mux_restore_characterization_test.go
internal/mux/mux_restore_test.go
internal/mux/mux_restore_wiring_test.go
internal/mux/restore_coordinator.go
internal/mux/restore_coordinator_test.go
scripts/check-maturity-gates.go
```

## Atomic commits

| Class | Commit | Purpose |
|---|---|---|
| T | `bec3126` | `test(mux): characterize restore transaction parity`; pin publication, rollback, identity, retained defects and performance paths. |
| A | `d073df1` | `refactor(mux): add restore coordinator seam`; add the private unwired seam. |
| M | `fe91ce0` | `refactor(mux): split restore operation adapters`; mechanically split operation adapters. |
| W | `294b2f2` | `refactor(mux): wire restore coordinator`; wire unchanged public facades. |
| G | pending | `refactor(mux): guard restore coordinator delegation`; guards, benchmarks, evidence and minimal docs. |

The maturity gate fixes exact T/A/M/W identities, subjects and direct ancestry. In full history on the active pre-G branch, `HEAD` must equal immutable W, porcelain tracked/staged/nonignored-untracked paths must equal the exact nine G-stage files, and the union of `36450fe..294b2f2` plus those paths must equal the full exact allowlist. A history-limited active pre-G checkout enforces the same exact W and exact nine-path porcelain contract: every required G file must be present and no outside path is allowed. Once exact-subject G exists on the active branch, `HEAD` must equal G and the nonignored worktree must be clean. G must expose the exact sole parent to be W. Merge/main history validates only the immutable `36450fe..G` range; later unrelated commits are outside Slice 6.2c. A shallow checkout also validates the documented sequence/allowlist, rejects wrong-parent G metadata, and fails the active slice when parentage cannot be proved. Later-main shallow state remains allowed.

## Static and runtime guards

- The coordinator remains a private, import-free, zero-field generic struct with exactly two parameters constrained by the exact two private ports.
- Port budget is exactly 5: preparation has exactly two methods and publication exactly three; all five names and full signatures are fixed, with no owner/state bags.
- The coordinator has exactly five production methods with exact receivers, signatures and one-call bodies; its constructor has the exact zero-value body.
- Recursive aliases, pointers, arrays, slices, maps, anonymous/named containers, retained adapters, inferred composites, local inferred containers, function-literal parameter/result/body retention, escaping adapter captures, exported seams and direct-adapter/controller bypasses are rejected in production scope; tests are excluded from this retention scan.
- `Mux` contains exactly one exact coordinator field and initializer. Exactly five facade-to-controller calls and exactly five controller-to-port calls are fixed; the five public signatures and one-call bodies are exact.
- Normal restore publication keeps reader preparation and palette setup before publication; exact live top-level PaneStarted/geometry construction and activation/focus append follow model/metrics/bounds/bootstrap/pending/commit publication and precede reader launch. The complete adapter body is fingerprinted to immutable W. Reverse abort clears the pending candidate before closing panes in descending acquisition order.
- Exactly one Slice 6.2c facade TODO remains. The exact two restore known-defect tests, signatures, expiry slices and canonical behavior-body hashes are pinned to immutable T when available and embedded fallback fingerprints otherwise; build-tag exclusion, `t.Skip`, no-op and behavior mutations reject.
- Synthetic guard self-tests reject nested aliases/containers, function-literal and closure retention, alternate fields, retained adapters, direct bypasses, exported seams, missing shallow contracts/allowlist entries, dirty/staged/untracked outside pre-G paths, missing required G paths, wrong-parent G metadata, active post-G descendants, dirty post-G worktrees and restore-order dead-code/helper/event mutations while allowing later-main shallow state.

## Same-host performance evidence

Windows/amd64, AMD Ryzen 9 7940HX, Go 1.25.8. The base is additive seam A `d073df1`; the canonical candidate binary is immutable W `294b2f2` with exact W production and test source. The canonical capture uses exactly ten interleaved 2-second, `-cpu=1`, `-count=1`, `-benchmem` samples per identity and benchmark after one unrecorded one-second warm-up per binary. It covers prepare-abort, window IDs, fresh snapshot, invalid/pending fast paths, Phase 15 many-tabs/windows snapshot, and inherited all-disabled image, ingress, protocol and scheduler paths. The separately rerun attribution binary uses exact W production plus the retained G test-only benchmark extension; its source and binary hashes and all raw samples are recorded. Allocations are blocking.

Canonical ten-sample medians are:

| Benchmark | Base | Candidate | Delta | Allocation disposition |
|---|---:|---:|---:|---|
| Restore prepare-abort | 156,967.5 ns | 152,370.5 ns | -4,597.0 ns, -2.93% | 640,560 B/op, 193 allocs/op unchanged |
| Restore window IDs | 26.450 ns | 26.970 ns | +0.520 ns, +1.97% | 16 B/op, 1 alloc/op unchanged |
| Fresh-session snapshot | 2,575.5 ns | 2,523.5 ns | -52.0 ns, -2.02% | 2,064 B/op, 36 allocs/op unchanged |
| Invalid restore fast path | 0.68935 ns | 2.8475 ns | +2.1582 ns, +313.07% | 0 B/op, 0 allocs/op both |
| Pending restore fast path | 2.2460 ns | 5.9725 ns | +3.7265 ns, +165.92% | 0 B/op, 0 allocs/op both |
| Phase 15 many-tabs/windows snapshot | 5,452.0 ns | 5,303.5 ns | -148.5 ns, -2.72% | 7,714 median B/op, 152 allocs/op unchanged |
| All-disabled image idle | 30.685 ns | 30.955 ns | +0.270 ns, +0.88% | 0 B/op, 0 allocs/op both |
| Ingress ASCII | 11,383.0 ns | 11,300.5 ns | -82.5 ns, -0.72% | 3 allocs/op unchanged; +0.5 median B/op noise with lower candidate maximum |
| Protocol dispatch idle | 12.685 ns | 11.770 ns | -0.915 ns, -7.21% | 0 B/op, 0 allocs/op both |
| Protocol completion discard | 20.275 ns | 20.445 ns | +0.170 ns, +0.84% | 0 B/op, 0 allocs/op both |
| Protocol Drain idle | 125.100 ns | 120.800 ns | -4.300 ns, -3.44% | 0 B/op, 0 allocs/op both |
| Work scheduler | 289.850 ns | 291.200 ns | +1.350 ns, +0.47% | 0 B/op, 0 allocs/op both |

The two restore fast-path percentages above 3% are not threshold passes. Their absolute costs are +2.1582 ns and +3.7265 ns. The exact ten-round retained-source rerun covers six direct-adapter/coordinator pairs: fresh-invalid +145.750 ns, invalid-window-IDs +2.1390 ns, pending-prepare +0.9775 ns, window-IDs +0.790 ns, commit-invalid +0.9745 ns, and abort-invalid +0.1325 ns. Candidate direct-adapter staging explains +3.3740 ns of the pending path before the +0.9775 ns coordinator step; the staged/full 0.6250 ns mismatch is explicitly retained as measurement/composition noise. The noisier allocation-heavy fresh pair is not used to explain a canonical threshold. No allocation count increased, exact restore B/op is unchanged, and no unrelated regressing median exceeds 3%. All ten raw samples for every attribution subbenchmark, source/binary identities, commands, and recomputable medians are in `architecture-maturity-slice-6.2c/benchmarks-candidate.txt`; canonical base/candidate arrays remain in `benchmarks-{base,candidate}.txt`.

## Verification disposition

Exact commands/results are recorded in `architecture-maturity-slice-6.2c/gates.txt`; raw benchmark samples are in `benchmarks-{base,candidate}.txt`; immutable scope/commit evidence is in `scope-and-commits.txt`. Windows real-GUI qualification is `UNRUN`: this preparatory slice does not change window/resource ownership, renderer behavior, geometry, pixels, feature defaults or support claims. No frontend, layoutstate, model, registry or image production source changed in G. No release, changelog, deployment or commit is produced.
