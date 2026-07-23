# CervTerm Phase 15 Context — Integration and Release Hardening

Date: 2026-07-23
Production baseline: `main` merge `1a42be8`, checkpoint `v0.9.0-beta.1`.

Objective: complete cross-feature integration, migration evidence, capability diagnostics, recovery/redaction proof, reproducible performance comparison, platform qualification, release gates, and honest close-out without broadening support claims beyond evidence.

Required boundaries: no renderer selection or remote domains; no automatic config rewriting; no persistent image cache; no live/native doctor side effects; no default-on experiments; no SKIP/UNRUN promoted to PASS; no publication without separate explicit approval. Image-cache recovery means transient context-local cache/activation rollback.

Known gaps: real-user migration evidence, unified platform doctor output, Phase 11 support-matrix drift, broader performance harness, consolidated recovery/redaction evidence, and platform qualification.

Authority: ADR-0002, ADR-0007, ADR-0008, ADR-0014, ADR-0016, Phase 15 preflight/guardrails, and the independently verified Phase 15 design/plan.
