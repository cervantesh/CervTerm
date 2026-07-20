# Phase 10.4a Validation — Semantic History Query

This slice prepares semantic navigation without exposing actions yet.

## Contract

- A packed marker bit in the existing one-byte semantic cell metadata distinguishes repeated same-kind shell boundaries without changing the 32-byte `Cell` budget.
- Blank semantic lines use a metadata-only cell sentinel. No rune, command text or shell property is synthesized or retained.
- Row-leaving through newline, cursor movement and reverse index finalizes a blank semantic line; autowrap and nonblank rows retain their existing cell metadata.
- `Terminal.SemanticHistory` scans directly from the scrollback ring and live rows newest-first. It never clones the full buffer before applying its one-million-cell budget.
- A single row wider than the cell budget returns a truncated empty result. Results retain the newest 4096 ranges, report cell/range truncation, preserve explicit marker starts, and return in chronological order.
- Global physical-row coordinates are snapshot-relative, matching existing search/viewport conventions. Mux snapshots carry pane/focus plus content, reflow and viewport generations; consumers must reject stale coordinates before acting.
- Mux snapshots own detached range slices and introduce no frontend, action, Lua, native or external-effect dependency.

## Evidence

Tests cover multiline output with blank rows, same-kind marker separation, newest-range retention, oldest cell-budget truncation, oversized rows, ring/live global coordinates, detached mux values and generation invalidation. Independent audit reported no blocker or important findings after remediation.
