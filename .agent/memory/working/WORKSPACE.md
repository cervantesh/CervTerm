# Workspace (live task state)

## Current task
Execute CervTerm WezTerm-parity Phase 13 one slice at a time. Slice 13.4 dormant edit/scroll/history image lifecycle is implemented, independently reviewed to PROCEED, and ready to commit/local merge.

## Open files
- `internal/core/images_lifecycle.go` private placement erase/move/drop/projection helpers
- Core erase/edit/scroll/history/capacity mutation seams and exact reservation retirement
- Exhaustive lifecycle boundaries, ring wrap, viewport pinning, stale preparation and 1,024-placement tests
- Nil/default production dormancy and text/store performance gates

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

## Next step
Commit Slice 13.4, merge it into local `dev`, then advance automatically to Slice 13.5 reflow/alternate/reset lifecycle and public core image API review.
