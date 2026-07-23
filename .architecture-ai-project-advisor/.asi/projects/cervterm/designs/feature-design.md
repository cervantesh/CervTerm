# Feature Design — Phase 15 Integration and Release Hardening

## Intent

Produce a reproducibly qualified, migration-aware, recovery-safe release candidate from the Phase 1–14 implementation set without overstating experimental support.

## Evidence contract

- `PASS`: named commit/host/config, all assertions successful, complete evidence.
- `FAIL`: attempted row with any failed assertion.
- `SKIP`: attempted row whose named prerequisite was unavailable, with prerequisite evidence.
- `UNRUN`: not executed; never supports promotion.
- `NOT-APPLICABLE`: named architecture/platform exclusion; not execution success.

## Components

1. Machine-checkable authority/evidence inventory.
2. Static side-effect-free doctor capability projection: intent, build availability, activation not probed, manual qualification, support claim.
3. Sanitized real v1 migration examples and paired v2 semantic goldens through the existing read-only loader.
4. Consolidated config/layout/transient-image recovery and redaction qualification.
5. Same-host reproducible performance harness. Phase 13/14 comparables retain exact ten-sample/3% gates; older Phase 0 comparables retain the 15% investigation threshold; allocations block.
6. Windows packaged qualification plus Linux/macOS build/headless matrices and honest GUI SKIP/UNRUN evidence.
7. CodeQL, govulncheck, race, accessibility privacy and output-redaction gates.
8. Release readiness checks with a separate explicit publication approval boundary.

## Data and behavior boundaries

No persistent cache, migration writer, live doctor probe, new support-state database, renderer direction or domain abstraction. Diagnostics are detached and side-effect free. Recovery publishes old-or-new. Existing ADR-0002 provenance paths remain governed by its redaction policy.

## Rollback

Each slice is independently revertible. Experimental flags remain false; status/evidence changes require no data migration.

## ADR determination

No new ADR while Phase 15 stays within the preflight boundaries.
