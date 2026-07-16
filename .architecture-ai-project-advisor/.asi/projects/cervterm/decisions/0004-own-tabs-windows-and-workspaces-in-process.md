# ADR: Own tabs, windows, and workspaces in process

## Status
Proposed

## Date
2026-07-16

## Context
The mux owns one implicit tab and independent pane aggregates. Visible tabs, multiple native windows, and named workspaces need stable identity and lifecycle without introducing WezTerm-style domains, a daemon, or live detach/reattach.

## Decision to Make
Define ownership among the process controller, native windows, mux windows, tabs, split trees, and panes before Phases 8–9.

## Candidate Direction
`AppProcess -> WindowController -> MuxWindow -> Tab -> SplitTree -> Pane -> local PTY session`. The mux owns identity, order, topology, focus, and lifecycle; the frontend owns native handles, chrome projection, and hit testing.

## Constraints
- GLFW/OpenGL calls stay on the OS thread.
- No local/SSH/WSL/remote domain abstraction.
- Existing one-tab behavior remains compatible.
- Moving a pane preserves exactly one session owner.
- Inactive tabs do not receive input or unnecessary rendering.

## Evidence Required for Acceptance
Ownership diagram, lifecycle table, move/close failure behavior, OS-thread model, and one-window compatibility path.

## Reversal Signals
The window toolkit cannot support multiple windows under one serialized owner, or pane movement cannot preserve lifecycle safely.
