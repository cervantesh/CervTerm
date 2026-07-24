# CervTerm Architecture Maturity Remediation Context

Date: 2026-07-23
Production baseline: `e9f9b2c` (`v0.10.0-beta.1`).

Objective: close the 30 accepted maturity findings and continue evidence-driven remediation until both independent audit teams score every architecture dimension at least 8.0.

Execution is one branch/PR/merge at a time from current `origin/main`. Per developer direction, work begins with parity-only preparatory extraction of `App`, then `Mux`, then `fontglyph`. App and Mux findings remain partial until their behavioral and ownership dependencies merge; their final thin-facade slices close them later.

Required boundaries: preserve renderer selection exclusion, v1/v2 config compatibility, default-off experiments, stable identities, fresh-session persistence, bounded resources, redaction, and locked OS-thread GLFW/OpenGL ownership. Never mix behavior correction with movement. Never use the dirty `fix/windows-version-resource-from-tag` worktree.

Architecture direction: explicit process/mux owner capability; process-owned shared config with window-owned geometry; schema/catalog authorities for closed vocabularies; parser-owned framing events projected into public output; compatibility facades over acyclic controller/font subpackages.

Evidence authority: the 2026-07-23 maturity review, accepted ADRs 0017–0021, versioned scoring protocol, Phase 0 baselines, current guardrails, and the reviewed remediation implementation plan.
