# Phase 10.4b3 Validation — Select Semantic Zone

## Contract

- `SelectSemanticZone("input"|"output")` is a closed serializable pane-targeted action with registry, codec, Lua and Teal parity.
- It uses the same generation-checked current prompt-cycle lookup and fresh terminal-range validation as semantic copying.
- Before any mutation, the executor proves the complete physical range fits one viewport and computes the clamped target viewport. Unsupported cross-viewport ranges fail without scrolling, clearing, or partially replacing an existing selection.
- Successful actions scroll through mux ownership and project exclusive semantic endpoints to inclusive pane-local selection endpoints. Focus and sibling pane state are unchanged.
- Render snapshots retain detached row-wrap flags. Selection text suppresses newlines after soft-wrapped physical rows while preserving hard breaks, matching `CopySemanticZone`.
- The action selects inert cells only; it does not copy, paste, launch or execute shell content.

## Evidence

Tests cover action/codec/registry/Lua validation, focused current-cycle selection, selected text extraction, soft-wrap equivalence, cross-viewport rejection before viewport/selection mutation, and missing-zone errors. Selection package tests pin mixed soft/hard row behavior while preserving legacy `Text` behavior.
