# Architecture Preflight — Maturity Remediation

Date: 2026-07-23
Baseline: `origin/main` at `e9f9b2c`
Decision: **PROCEED WITH CONSTRAINTS**

## Scope

Thirty accepted findings across architecture maturity, Clean Code, GRASP, dependency hygiene, domain isolation, ownership/transactions, and test/guardrail maturity. Work continues until every accepted finding is closed and each independently scored dimension is at least 8.0.

## Locked execution

1. Phase 0 evidence and ADRs.
2. Preparatory, behavior-preserving `App`, `Mux`, then `fontglyph` extraction.
3. Ownership, multi-window, trust/security, lifecycle, API and authority repairs.
4. Formal thin-Mux/thin-App closure only after semantic dependencies.
5. Two-team/two-round scoring loop until every row passes.

## Accepted decisions

- ADR-0017: explicit process/mux owner capability and fail-closed wrong-owner behavior.
- ADR-0018: process-owned shared config transaction and per-window geometry ownership.
- ADR-0019: generated/schema-owned closed authorities.
- ADR-0020: parser framing decisions project public output.
- ADR-0021: compatibility facades and acyclic controller/font package extraction.

## Preconditions and stop conditions

Characterization precedes movement. `T`, `A`, `M`, `W`, and `G` remain distinct commits. Known-defect goldens carry finding IDs and expiry slices. Stop on behavior drift during a move, package cycles, ownership ambiguity, partial publication/recovery, sensitive leakage, unexplained performance/allocation regression, renderer-selection expansion, dirty-worktree contamination, or a new durable boundary without ADR review.

The App/Mux preparatory slices may create only private delegation seams under existing entry points. They cannot transfer final ownership, expose bypass ports, copy mutable state, or close L1-01/L3-01. The font extraction may proceed only under ADR-0021's exact package DAG and lease ownership.

## Required evidence

Clean current-main baseline; all-tracked-production-file package graph; focused/full/race/tagged/recovery tests; Phase 15 performance captures; two-window geometry/lifecycle trace plus explicit absence of process-owned shared-config generation; accessibility/public-output goldens; CI and CodeQL. Repeat preflight before ownership, security-sensitive, and final controller stages.
