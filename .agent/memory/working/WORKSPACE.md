# Workspace (live task state)

## Current task
Execute CervTerm WezTerm-parity Phase 13 one slice at a time. Slice 13.1 is implemented, fully validated, and independently approved; commit and local merge are next.

## Open files
- `internal/vt/parser.go`, `parser_esc.go`, and `parser_control_string.go`
- `internal/vt/parser_control_string_{test,benchmark_test}.go`
- `docs/validation/phase-13-control-string-baseline.{md,txt}`
- `scripts/capture-phase13-benchmark.go` control suite
- Phase 13 implementation plan and portable baseline gates

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
- [ ] Commit and locally merge Slice 13.1.

## Next step
Commit and merge Slice 13.1 into local `dev`, then advance automatically to bounded process/pane store foundations in Slice 13.2.
