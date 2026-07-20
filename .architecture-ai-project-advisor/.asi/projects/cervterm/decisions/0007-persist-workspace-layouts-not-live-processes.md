# ADR: Persist workspace layouts, not live processes

## Status
Accepted

## Date
2026-07-16

## Context
Named local workspaces should restore useful organization without introducing a daemon, domains, live-session continuity, or serialization of PTY/process handles.

## Decision to Make
Define the versioned persistence format, restore semantics, migration, corruption behavior, and privacy boundary before Phase 9.

## Candidate Direction
Persist window bounds, workspace names, tab order, split ratios, cwd/launch descriptors, and appearance overrides. Restore spawns new local sessions and explicitly does not restore live processes, scrollback by default, credentials, or handles.

## Constraints
- Persistence is non-authoritative and safe to delete.
- Old/corrupt state fails to a fresh window.
- Paths and environment data follow explicit privacy/redaction policy.
- Off-screen window bounds recover to active monitors.
- No local/SSH/WSL domain abstraction.

## Evidence Required for Acceptance
Schema example, migration policy, corruption tests, monitor/DPI recovery, privacy review, and user-visible restore wording.

## Reversal Signals
Layout-only restore is misleading or provides insufficient value without live sessions.

## Migration and Privacy Policy

Version 1 is the first persisted format. Missing, malformed, unsafe, or unknown/future versions are non-authoritative and fall back without mutation; a future version must add an explicit pure migration into the current validated plan before side effects. Environment maps, dedicated credential fields, handles, process state, runtime IDs, terminal contents, and scrollback are structurally absent. Local program arguments remain trusted user-authored launch data and are not claimed to be secret-redacted.

## Acceptance Evidence

Implemented and validated through PR #202: strict version/limit/corruption tests, atomic store rollback, monitor/DPI recovery, fresh-session transaction rollback, structural exclusion tests, safe fallback wording, and save→load→restore coverage. See `docs/validation/phase-9-windows-workspaces-layout-persistence.md`.
