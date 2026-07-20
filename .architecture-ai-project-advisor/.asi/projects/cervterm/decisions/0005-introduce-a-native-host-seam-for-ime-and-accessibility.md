# ADR: Introduce a native host seam for IME and accessibility

## Status
Proposed

## Date
2026-07-16

## Context
GLFW character callbacks do not model composition/preedit or native accessibility trees. CervTerm needs platform services without leaking Windows, Cocoa, X11, Wayland, or GLFW types into core, input, render, or mux packages.

## Decision to Make
Define the smallest native host interface and platform ownership needed for Phases 11–12.

## Candidate Direction
A frontend host seam exposes composition start/update/commit/cancel, candidate rectangle, focus/caret notifications, and accessibility snapshot/event adapters. Preedit remains frontend state; only committed text reaches the terminal encoder.

## Constraints
- GLFW/OpenGL remain main-thread/OS-thread bound.
- Preedit never reaches PTY before commit.
- Accessibility consumes immutable snapshots and bounded/coalesced events.
- Windows ships first; unsupported platforms report capability explicitly.
- Core and mux remain native-toolkit neutral.

## Evidence Required for Acceptance
Windows API spike, lifecycle/state diagrams, duplicate-character suppression tests, accessibility role/text-range mapping, and platform fallback behavior.

## Reversal Signals
Required native hooks cannot coexist with GLFW, forcing a separately approved window-host replacement.
