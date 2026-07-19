# Phase 10.4b2 Validation — Copy Semantic Zone

## Contract

- `CopySemanticZone("input"|"output")` is a closed serializable pane-targeted action with registry, codec, Lua and Teal parity; other values fail during configuration/action validation.
- The current command cycle is anchored by the most recent prompt at the viewport top, or at the cursor while the viewport is at the bottom. The requested input/output range must occur before the next prompt.
- Core text extraction is capped at one MiB, includes Unicode combining marks once, skips wide continuations and metadata-only blank sentinels, preserves hard newlines, and removes physical newlines introduced solely by soft wrap/reflow.
- Mux validates pane/focus/content/reflow/viewport generations and verifies the target against both the detached snapshot and freshly derived terminal history. Mutating both a detached range and its snapshot cannot forge access.
- Clipboard mutation occurs only after all validation and extraction succeeds, on the frontend thread behind the existing injectable clipboard seam.
- Copied shell text is never parsed, launched, pasted or executed by this action.

## Evidence

Tests cover action/codec/registry/Lua validation; Unicode, wide and combining text; blank semantic lines; hard versus soft-wrap/reflow newlines; invalid ranges; forged detached snapshots; stale generations; current-cycle input/output copying; missing-zone failure; and clipboard non-mutation on failure.
