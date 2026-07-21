# Workspace (live task state)

## Current task
Execute CervTerm WezTerm-parity Phase 13 one slice at a time. Slice 13.6 detached render/mux image projection and independent damage identity are implemented, independently reviewed to PROCEED, and ready to commit/local merge.

## Open files
- Reusable core placement/crop projection into renderer-neutral snapshots
- Stable pane-object plus image-generation damage identity with text-only row hashes
- Recursive public PaneView detachment including hidden scratch, combining cells and crops
- Literal-nil/default text path with zero image allocations

## Active constraints
- Preserve 32-byte text-only `core.Cell`.
- Keep terminal image bytes/counts bounded per pane and process.
- OpenGL remains the only renderer direction; renderer selection is excluded.
- Dirty primary worktree `fix/windows-version-resource-from-tag` must remain untouched.
- Execute, validate, commit, and locally merge each slice before the next.

## Checkpoints
- [x] Phase 13 preflight, ADR, design, plan, and independent PASS.
- [x] Hermes-only Agentic Stack onboarding merged into `dev`.
- [x] Slice 13.0a architecture authority merged into `dev`.
- [x] Slice 13.0b full/tagged/race/vet/fuzz/import/maturity gates run.
- [x] Stable warmed single-P ten-sample baselines recorded; strict malformed/identity/allocation negative cases reject.
- [x] Independent Slice 13.0b review reached PROCEED after all harness-identity blockers were closed.
- [x] Slice 13.0b merged into local `dev` at `4abc9f8`.
- [x] Baseline portability repair normalizes source identity, regenerates both baselines, and has independent PROCEED.
- [x] Baseline portability repair merged into local `dev` at `1650bc6`; Slice 13.1 rebased successfully.
- [x] APC/DCS framing, bounds, reset/EOF, split/overflow tests, fuzz smoke, and zero-allocation first-result benchmarks implemented.
- [x] Independent reviews closed repeated-ESC, overlapping-ST, fuzz breadth, lazy-state, CSI geometry, and pending-wrap findings with final PROCEED.
- [x] Final full/tagged/vet/race/maturity/import gates and both mandatory 60-second fuzz targets pass; minimized CSI corpus retained.
- [x] Isolated text/control performance captures pass the 3% and worst-allocation gates; portable first-result control baselines are recorded.
- [x] Slice 13.1 merged into local `dev` at `fb97fd6`.
- [x] Slice 13.2 normative limits, ownership, rollback, concurrency, and cross-slice API restraints analyzed.
- [x] Hard/lower-only limits, atomic pane/process reservations, autonomous bounded transfer expiry/removal, detached acquisition, decoded candidate leases, epochs and generation preparation implemented.
- [x] Independent review findings for unbounded pending retention, passive expiry, candidate lifecycle, generation mutation, exact placement caps, and source identity are closed with final PROCEED.
- [x] Full/tagged/vet/race/maturity/import gates and both new 60-second termimage fuzz targets pass.
- [x] Isolated text/control/store performance gates pass; store first-result baseline and documented control recalibration are recorded.
- [x] Slice 13.2 merged into local `dev` at `b98b526`.
- [x] Slice 13.3 strict placement/crop/delete-selector contracts, primary/alternate sidecars, prepared complete store states, exact candidate/resource/placement ownership, owner-thread two-pointer publication, reset/close cleanup, and generation exhaustion are implemented.
- [x] Adversarial reviews closed stale prepared state, candidate alias/close races, exact-cap placement replacement, CurrentScreen resource scope, reset/close placement leaks, and sidecar generation wrap; final independent verdict is GO.
- [x] Full and focused race suites, placement/delete/store fuzz targets, import/Cell/default-dormancy checks, and isolated text/store performance gates pass; full vet reports only pre-existing DirectWrite unsafe.Pointer diagnostics.
- [x] Slice 13.3 committed at `d49fa47` and locally merged into `dev` at `053f8a0`.
- [x] Slice 13.4 whole-placement overwrite/erase/edit/scroll/history/ED3/capacity lifecycle and scrolled-back projection are implemented while creation remains private/unreachable.
- [x] Independent reviews closed horizontal clipping, combining-cell/right-margin/wide-span lifecycle, stale preparation cleanup, lazy no-op allocation, boundary/entry coverage and bounded 1,024-placement findings with final PROCEED.
- [x] Full/tagged/vet/race/maturity/import/fuzz smoke and isolated text/store performance gates pass; ASCII width and repeated-blank fast paths keep disabled text operation allocation-free and below baseline.
- [x] Slice 13.4 committed at `0f43543` and locally merged into `dev` at `c45fa52`.
- [x] Slice 13.5 deterministic primary reflow, padding-anchor retention, callback-free ring rebuild, alternate crop/exit and RIS reset are implemented.
- [x] StoreOwner gates every prepared mutation/publication; reset atomically drains resources, transfers, placements and loose decoded candidates with no insertion race.
- [x] Public core image API is externally testable, owner-thread documented, detached and bounded; no production caller attaches it before mux/runtime slices.
- [x] Independent review reached PROCEED after ownership bypass, reset race/zero-usage, padding mapping, shared boundary map, coverage and maturity findings were closed.
- [x] Full/tagged/vet/race/maturity/import/fuzz-smoke and isolated text/store performance gates pass.
- [x] Slice 13.5 committed at `284865e` and locally merged into `dev` at `4d08bec`.
- [x] Slice 13.6 snapshots carry only pane-qualified placement/resource references and image generation; no pixels, stores, leases or GL handles.
- [x] Render capture reuses placement/crop capacity; Mux PaneView recursively detaches every public slice and nested crop/combining value.
- [x] Text row hashes remain image-free; pane-qualified image generation independently damages both back buffers without pinning redraw.
- [x] Independent review reached PROCEED after buffer-age, pane identity, nil semantics, crop allocation and hidden scratch findings were closed.
- [x] Full/tagged/vet/race/maturity/import and isolated text performance gates pass with 0 B/op and 0 allocs/op.

## Next step
Commit Slice 13.6, merge it into local `dev`, then advance automatically to Slice 13.7 mux store lifecycle, resource acquisition and bounded shared replies.
