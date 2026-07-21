# Workspace (live task state)

## Current task
Execute CervTerm WezTerm-parity Phase 13 one slice at a time. Slice 13.0b is merged; a portability repair closes LF/CRLF harness identity drift before Slice 13.1 continues.

## Open files
- `docs/validation/phase-13-baseline.{md,txt}` and `phase-13-gl-baseline.txt`
- `scripts/check-phase13-imports.go`
- `scripts/compare-phase13-baseline.go`
- `scripts/capture-phase13-benchmark.go`
- Phase 13 benchmark tests in VT/render/GLFW and exact Cell invariant

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
- [ ] Commit and locally merge the portability repair, then rebase the in-flight Slice 13.1 worktree.

## Next step
Commit and merge `fix/phase-13-baseline-portability`, rebase Slice 13.1 onto updated `dev`, and continue APC/DCS framing without skipping its validation gates.
