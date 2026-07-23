# Implementation Plan — Phase 15 Integration, Performance, Migration, and Release

Baseline: `main` `1a42be8`; incoming checkpoint `v0.9.0-beta.1`.

## Sequence

1. **15.0 Authority/evidence:** sync context, preflight, guardrails, design and plan; add normative evidence JSON and maturity validation.
2. **15.1 Support/doctor:** correct Phase 11 IME status; publish static build-tagged capability rows and path/value-free side-effect-free doctor output; preserve ADR-0002 provenance diagnostics.
3. **15.2 Migration:** add sanitized real v1 examples, paired v2 templates, semantic goldens and source-immutability tests. Never invent a real-user PASS.
4. **15.3 Recovery/redaction:** fault-inject config last-known-good, layout fresh fallback, transient image cache/activation rollback and bounded panic/log classification.
5. **15.4 Performance:** add startup/memory/idle/input/resize/font/tabs/windows/semantic/image harness. Use exact inherited ten-sample/3% Phase 13/14 protocol; other Phase 0 comparables use >=3/15%; allocations block.
6. **15.5 Qualification/security/accessibility:** Windows package/daily-driver, Linux/macOS build/headless, CodeQL, govulncheck, race/privacy suites; unavailable GUI/manual rows remain SKIP/UNRUN.
7. **15.6 Release readiness:** assert experimental defaults, matrix/docs consistency, recovery evidence, package exclusions/checksum inputs; no publication without explicit approval.
8. **15.7 Close-out:** rerun full/tagged/race/vet/security/maturity/import/package gates after repairs; write drift/close-out and update only justified claims.

## Global success

Every roadmap item maps to PASS or an explicitly allowed non-promoting disposition; security/accessibility blockers are absent; comparable performance stays within inherited budgets or has explicit accepted analysis; package gates pass; experimental/default-off/no-claim boundaries are mechanically enforced.

## Stop conditions

New ADR direction, source rewriting, persistent image cache, live doctor effects, partial recovery, sensitive logs, support promotion from non-manual evidence, weakened performance protocol, unexplained regression or required gate failure.
