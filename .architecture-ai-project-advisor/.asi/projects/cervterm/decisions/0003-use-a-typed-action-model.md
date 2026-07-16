# ADR: Use a typed action model

## Status
Proposed

## Date
2026-07-16

## Context
Bindings currently combine Lua callbacks, GLFW normalization, and fixed frontend handlers. Tabs, windows, mouse bindings, command palette, quick select, and launch menu require one discoverable dispatch seam without moving toolkit types into terminal or mux layers.

## Decision to Make
Choose action identity, arguments, serialization, targeting, labels, callback compatibility, and modal precedence before Phase 1 implementation.

## Candidate Direction
Add a toolkit-neutral action registry with typed arguments, metadata, target resolution, and an explicit bounded Lua-callback action. Precedence is modal UI, safety/reload, user binding, built-in, then PTY encoding.

## Constraints
- Existing shortcuts and Lua callbacks remain compatible.
- Actions do not import GLFW types.
- Target pane/tab/window is resolved at dispatch time.
- Validation errors identify the action and bad argument.
- Callback watchdog and main-thread execution remain intact.

## Evidence Required for Acceptance
Representative action schema, serialization examples, precedence table, callback failure semantics, and pane-origin/active-pane targeting tests.

## Reversal Signals
Typed serialization prevents required Lua behavior, or a smaller command interface provides equal validation and discovery.
