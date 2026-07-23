# Phase 15 Integration and Release-Hardening Preflight

Date: 2026-07-23  
Production baseline: `1a42be8` (`v0.9.0-beta.1`)

## Verdict

Proceed with constraints. Phase 15 is qualification, diagnostics, migration evidence, recovery proof, performance comparison, platform evidence, and release-gate work. It does not introduce a new runtime ownership or persistence model, so no new ADR is required while the boundaries below hold.

## Accepted authority

- Versioned configuration remains in-memory and never rewrites user source.
- Persisted layout remains non-authoritative and falls back to one fresh usable window.
- Image caches remain transient and context-local; recovery means old-or-new activation plus deterministic close/recreate, not a disk cache.
- `--doctor` remains read-only and cannot initialize GLFW, PTYs, providers, notification sinks, or live feature activation.
- Renderer selection and remote domains remain excluded.
- IME, accessibility, Kitty, Sixel, and iTerm remain restart-scoped/default-off experiments unless later evidence and a new approved decision justify promotion.
- Tagging, uploading, attestation, and publication require explicit approval after candidate gates pass.

## Normative evidence states

| State | Entry criterion |
|---|---|
| `PASS` | Executed on the named commit, host, and configuration; every required assertion succeeded; complete evidence is attached. |
| `FAIL` | Executed and at least one required assertion failed. Partial success cannot override this state. |
| `SKIP` | Execution was attempted, but a named prerequisite was unavailable and that prerequisite check is recorded. |
| `UNRUN` | The row was not executed. It cannot justify a support or default change. |
| `NOT-APPLICABLE` | A named architecture or platform exclusion proves the row cannot apply. It is not a successful execution. |

## Performance policy

Phase 13/14 all-disabled, idle, and frontend-comparable rows retain the exact ten-sample protocol and a 3% unexplained median-regression threshold. Historical Phase 0 comparable rows use at least three samples and the existing 15% investigation threshold. Allocation regressions block. New features absent at Phase 0 use explicit candidate-only budgets and are never presented as comparative PASS results.

## Security and privacy policy

Candidate CodeQL must contain no open high-severity candidate alerts. `govulncheck`, race suites, accessibility privacy tests, and notification/image output-redaction tests must pass. Capability diagnostics and recovery classifications are value-free. Existing explicit configuration provenance paths remain governed by ADR-0002; Phase 15 does not silently remove that contract.

## ADR triggers

Stop and create a new ADR before persistent image storage, automatic configuration rewriting, live native probing from doctor, default-on promotion, renderer/backend selection, remote domains, or a new distribution channel.
