# CervTerm Phase 15 Guardrails

## Required

- [ ] Work from clean production `main`; preserve unrelated dirty worktrees and user files.
- [ ] Migration is in-memory/read-only; source bytes and adjacent files never change.
- [ ] Doctor separates configured intent, build availability, runtime activation not probed, manual qualification, and support claim.
- [ ] Capability and recovery output is value-free; ADR-0002 provenance diagnostics retain their documented redaction contract.
- [ ] Config, layout and transient image activation recovery remains old-or-new and ownership-safe.
- [ ] Phase 13/14 comparable performance uses exactly ten samples and the inherited 3% unexplained-regression threshold; allocations block.
- [ ] PASS, FAIL, SKIP, UNRUN and NOT-APPLICABLE satisfy their normative entry criteria.
- [ ] IME, accessibility and image experiments remain default-off without qualifying evidence.
- [ ] Full/tagged/race/vet/security/maturity/import/package gates run after the final repair.

## Forbidden without new ADR/design

- [ ] Renderer/backend selection, remote domains, new infrastructure or distribution channel.
- [ ] Persistent image cache or automatic config rewriting.
- [ ] Live native/runtime probing from doctor.
- [ ] Treating headless, skipped or unrun evidence as real-GUI PASS.
- [ ] Publication, tag or upload without explicit approval.

## Stop conditions

Stop on partial recovery, sensitive logging, support/default widening, weakened performance protocol, unexplained material regression, failed required security/accessibility gate, or any new durable ownership/persistence direction.
