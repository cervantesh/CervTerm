# ADR: Own shared config by process and geometry by window

## Status

Accepted

## Date

2026-07-23

## Relationship

Extends ADR-0004 for native-window projection and ADR-0002 for multi-window candidate publication. It does not alter ADR-0002 v1/v2 decoding semantics.

## Context

Multiple native windows require one coherent configuration/runtime publication while bounds, DPI, padding, tab chrome and pane metrics remain window-specific. Global frontend geometry or per-window config reload can diverge siblings.

## Decision

The process owner prepares one candidate config/runtime/Teal/resource transaction and publishes a generation atomically to all windows or none. Windows retain detached effective projections and report activation failure before commit. Each mux window owns its renderer-neutral bounds and per-pane metrics; its native projection supplies DPI and client geometry through typed events. No package global stores active window geometry or active config.

## Consequences

Reload and startup require multi-window prepare/commit/rollback and failure injection. Geometry and config become separately testable authorities.

## Rejected alternatives

Independent per-window reload, one global geometry singleton, or frontend mutation of mux internals.

## Rollback

Disable multi-window publication, revert consumers then shared transaction; retain old single-window adapter until removal criteria pass.
