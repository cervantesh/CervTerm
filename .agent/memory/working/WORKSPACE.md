# Workspace (live task state)

## Current task
Execute CervTerm WezTerm-parity Phase 13 one slice at a time from local `dev`. Architecture gates are complete; begin with Slice 13.0a tracked architecture synchronization.

## Open files
- `.architecture-ai-project-advisor/.asi/projects/cervterm/` (Slice 13.0a target)
- External planning source: `.t50-project-flow/.asi/projects/cervterm/` in the primary workspace
- Phase 13 accepted ADR 0014, feature design, and implementation plan

## Active constraints
- Preserve 32-byte text-only `core.Cell`.
- Keep terminal image bytes/counts bounded per pane and process.
- OpenGL remains the only renderer direction; renderer selection is excluded.
- Dirty primary worktree `fix/windows-version-resource-from-tag` must remain untouched.
- Execute, validate, commit, and locally merge each slice before the next.

## Checkpoints
- [x] Phase 13 preflight, ADR, design, plan, and independent PASS.
- [x] Hermes-only Agentic Stack onboarding; doctor green.
- [ ] Commit onboarding prerequisite to local `dev`.
- [ ] Implement and merge Slice 13.0a.

## Next step
Commit the approved Hermes-only onboarding on a dedicated branch, locally merge it into `dev`, then create the clean Slice 13.0a worktree from updated `dev`.
