# ADR: Gate terminal-originated external side effects

## Status
Proposed

## Date
2026-07-16

## Context
OSC hyperlinks, shell semantic zones, bell policies, notifications, clipboard operations, and launch actions can turn untrusted terminal output into host-side effects.

## Decision to Make
Define validation, allow/deny policy, rate limits, focus behavior, user consent, and logging before Phase 10.

## Candidate Direction
Parse metadata separately from side effects. Terminal output may request a bounded notification or expose a validated URI, but only explicit policy/action dispatch can open links, write clipboard data, launch commands, or surface repeated notifications.

## Constraints
- No terminal sequence directly executes a command.
- URI schemes and payload sizes are validated.
- Notifications and bells are rate-limited and focus-aware.
- Logs redact sensitive content.
- Lua receives read-only metadata unless a user-configured action grants behavior.

## Evidence Required for Acceptance
Threat model, scheme/payload policy, rate-limit table, focus/consent UX, audit/redaction design, and malicious-sequence tests.

## Reversal Signals
Native platform constraints require stricter defaults or removal of a side effect.
