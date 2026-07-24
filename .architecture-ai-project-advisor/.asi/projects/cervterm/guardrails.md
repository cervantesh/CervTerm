# CervTerm Architecture Maturity Guardrails

Canonical LF SHA-256 is pinned by the implementation plan after each accepted update. `.gitattributes` enforces LF for this artifact.

## Required delivery discipline

- [ ] Start every slice from clean current `origin/main`; preserve unrelated worktrees, user files and source configuration bytes.
- [ ] Execute one slice/PR/merge at a time; no unmerged stacking.
- [ ] Structural extraction uses distinct passing `T → A → M → W → G` commits. Behavior-only repair uses `T → B` (plus `G` only when needed). Never mix behavior and movement.
- [ ] Record `execution_predecessor`, `semantic_depends_on`, changed-path allowlist and generated-path exceptions per slice.
- [ ] Tag defect-preserving characterization with finding ID and expiry slice; replace it when the correction lands.
- [ ] Preserve process-local mux identities, fresh-session persistence, v1/v2 compatibility, redaction, resource caps and default-off support claims.
- [ ] Keep GLFW/OpenGL/native window calls on the locked OS thread; keep core/VT/render/mux/config free of frontend imports.
- [ ] Require old-or-new publication, singular ownership, idempotent close, fault-injected rollback and race evidence at ownership boundaries.
- [ ] Require package-DAG checks and the exact ten-sample inherited 3% median/allocation performance protocol for extraction slices; allocation regressions block.
- [ ] Classify PASS, FAIL, SKIP, UNRUN and NOT-APPLICABLE honestly; headless/skipped/unrun evidence never becomes real-GUI PASS.
- [ ] Doctor remains detached and side-effect-free: no live native/runtime activation probe; output distinguishes configured intent, build availability, activation-not-probed, manual qualification and support claim.
- [ ] Publication, tag or upload requires separate explicit approval.
- [ ] Close all accepted findings at every severity and require both independent teams to score every dimension ≥8.0.

## Preparatory extraction limits

- [ ] Early App/Mux extraction is private delegation beneath existing facades, not formal finding closure.
- [ ] No exported bypass port, copied mutable state, service-locator facade/concrete, ownership transfer or observable change.
- [ ] Consumer-owned controller ports normally have at most five concern-specific methods; forbid `any`, `map[string]any`, generic callback bags, facade returns and mixed-concern method sets unless an ADR/static allowlist approves an exception.
- [ ] Controllers retain only concern-owned state, stable IDs and detached immutable snapshots.
- [ ] Public font subsystem packages follow ADR-0021's complete edge allowlist and never import the root `fontglyph` facade or one another; exact imports of private `internal/face` and listed stable leaves are allowed.

## Preserved product/security boundaries

- [ ] Configuration migration/reload never rewrites user source automatically.
- [ ] Layout persistence remains layout-only and fresh-session; image caches remain transient/context-local.
- [ ] Capability/recovery output is value-free; sensitive values and terminal payloads never enter logs, provenance or evidence.
- [ ] IME, accessibility and image protocols remain default-off unless separately qualified.

## Forbidden without new ADR/design

- [ ] Renderer/backend selection, remote domains, daemon infrastructure or new durable external effects.
- [ ] New persistence authority, automatic config rewriting, persistent image cache or default-on experimental capability.
- [ ] Broad interfaces without a demonstrated Protected Variations boundary and at least two real consumers.

## Stop conditions

Stop on unrelated changes, failed required gate, package cycle, boundary inversion, partial recovery/publication, sensitive leakage, unreadable persisted state, broadened support/defaults, weakened evidence classification, live doctor side effects, unapproved publication, unexplained allocation/performance drift, behavior changes in `M`, or any final audit score below 8.0 without a new remediation slice.
