# ADR: Project parser framing events into public output

## Status

Accepted

## Date

2026-07-23

## Relationship

Refines ADR-0014 and ADR-0016 only for framing/public-output authority. Their payload bounds, image ownership, adapter subsets and default-off activation remain authoritative.

## Context

Public output projection must omit selected terminal-image envelopes without duplicating the VT parser state machine or retaining raw sensitive payloads. Parallel framing logic risks semantic and security drift.

## Decision

The VT parser remains the single framing authority and emits bounded typed framing decisions/events while consuming bytes. Terminal mutation and public-output projection consume the same decision stream. Projection may retain only the bounded undecided prefix required for selector choice; it never stores selected payloads. Byte accounting remains raw and separate from projected data. Unknown/unselected sequences preserve existing bytes.

## Consequences

Parser APIs gain typed events and dual-oracle fuzzing. Projection no longer owns a second grammar.

## Rejected alternatives

A parallel parser, regex filtering, raw-ingress callbacks, or payload buffering for diagnostics.

## Rollback

Revert projection consumer then event wiring; preserve parser/fuzz characterization.
