# ADR: Gate terminal-originated external side effects

## Status
Accepted

## Date
2026-07-20

## Context
OSC hyperlinks, shell semantic zones, bell policies, notifications, clipboard operations, and launch actions can turn untrusted terminal output into host-side effects. Terminal/PTY output is untrusted and must not directly invoke native APIs or commands.

## Decision
Terminal parsing may create only bounded pane-local metadata or requests. External effects execute on the frontend OS thread behind fakeable adapters and centralized policy gates.

- Link opening requires a fresh explicit user activation and an absolute HTTP(S) URI with a validated ASCII authority. Other schemes fail closed.
- Notification requests default to disabled, require explicit live consent, obey focus and bounded rate policy, and lose freshness while no native projection exists.
- Every BEL remains lossless in core/mux/Lua; only audible, visual, and taskbar sinks may be focus-filtered or throttled.
- OS launch adapters use argv-only APIs. Diagnostics report bounded generic capability failures and never include URI query/payload or notification title/body.
- Core, VT, render, and mux remain free of OS dependencies; native resources are projection-owned and transactionally cleaned.

## Consequences
Metadata and effect slices remain independently reversible. Existing OSC 7/52 and BEL callback behavior remains compatible. Some WezTerm behavior is intentionally stricter: custom URI schemes remain disabled, and terminal-originated notifications remain explicit opt-in.

## Acceptance Evidence
Phase 10 qualification covers malformed, truncated, invalid-control and oversized OSC payloads; hyperlink identity/lifetime and explicit activation; semantic row/reflow/alternate-screen behavior; default-off notification consent/freshness/focus/rate policy; lossless bell callbacks; redacted diagnostics; Windows native adapter lifecycle; full/default/GLFW/vet/race gates; and explicit platform manual pass/skip boundaries. See `docs/validation/phase-10-closeout.md`.

## Reversal Signals
A safer platform capability model supersedes this policy, or all native effects are removed while harmless metadata remains.
