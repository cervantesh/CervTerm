# Architecture Preflight — Phase 15 Integration and Release Hardening

Date: 2026-07-23
Decision: **PROCEED WITH CONSTRAINTS; NO NEW ADR WHILE BOUNDARIES HOLD**

Phase 15 is diagnostics, qualification, migration evidence, recovery proof, benchmark, documentation, and release-gate work under ADR-0002, ADR-0007, ADR-0008, ADR-0014, and ADR-0016.

## Locked interpretations

- Config migration is in-memory and read-only.
- Layout state is non-authoritative and falls back to a fresh usable window.
- Image-cache recovery is transient context-local close/recreate or activation rollback; no disk cache.
- Doctor uses static detached capability descriptors and never establishes runtime activation.
- PASS, FAIL, SKIP, UNRUN, and NOT-APPLICABLE have distinct entry criteria.
- Experimental/default-off features retain their current support boundaries without real evidence.
- Release publication remains separately approved.

## ADR triggers

Persistent image storage, config rewriting, live native doctor probing, default-on promotion, renderer/backend selection, remote domains, or a new release channel.

## Required gates

Machine-checkable evidence, support-matrix consistency, source immutability, recovery fault injection, privacy/redaction tests, exact inherited ten-sample/3% performance checks, security/accessibility gates, platform matrices, package smoke, final drift review, and explicit approval before publication.
