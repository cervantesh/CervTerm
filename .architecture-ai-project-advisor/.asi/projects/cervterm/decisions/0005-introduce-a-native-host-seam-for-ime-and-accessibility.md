# ADR: Introduce a native host seam for IME and accessibility

## Status
Accepted

## Date
2026-07-20

## Context
GLFW character callbacks do not model composition/preedit or native accessibility trees. CervTerm needs platform services without leaking Windows, Cocoa, X11, Wayland, GLFW, HWND or IMM32 types into core, input, render or mux packages.

## Decision
Use a family of narrow, optional, projection-owned native capability interfaces in the GLFW frontend rather than one broad native-host API.

Phase 11 adds only a `compositionHost`: start/update/commit/cancel events, candidate/composition rectangle publication, explicit capability, and deterministic close. Preedit remains bounded frontend state. One stable target activation is captured at composition start; target/focus/lifecycle drift cancels and never transfers composition. Only a target-validated exactly-once commit router may send text to modal/search or one pane/PTY.

The Windows implementation may transactionally subclass the GLFW HWND WndProc on the locked OS thread. It must strongly own and panic-contain the callback, chain every unhandled message to the exact prior procedure, pair IMM context acquire/release, bound/validate UTF-16 and composition attributes, and restore before projection unbind/HWND destruction. Disabled, unsupported or failed installation preserves the existing GLFW character path.

Phase 12 accessibility will introduce a separate optional capability and its own snapshot/privacy/event decisions; Phase 11 does not predesign it.

## Constraints
- Core, VT, input, render and mux remain native-toolkit neutral.
- Preedit never enters PTY, cells, snapshots, scripting or persistence.
- Modal/search use stable activation IDs distinct from mutable content revisions.
- Echo suppression precedes all character routing and is bounded by result generation/sequence/deadline.
- Candidate geometry uses checked framebuffer/window-size ratios and current target metrics; content scale is only invalidation.
- Native teardown is cancel -> deactivate callbacks -> restore WndProc -> release host/context -> projection unbind -> remaining resources -> HWND destroy.
- `ime.enabled` is restart-scoped and initially defaults false; default-on requires separate real Windows qualification.

## Acceptance Evidence
Windows API feasibility and lifecycle design, bounded state/target/geometry rules, explicit slice plan and rollback, and same-engine adversarial design/plan challenges are recorded in `docs/validation/phase-11-preflight.md` and T50 Project Flow artifacts. Implementation acceptance additionally requires duplicate suppression, lifecycle, geometry, native adapter and real Windows J/C/K evidence.

## Reversal Signals
Required native hooks cannot chain/restore GLFW deterministically, committed text cannot be made exactly once, or platform constraints require a separately approved window-host replacement.
