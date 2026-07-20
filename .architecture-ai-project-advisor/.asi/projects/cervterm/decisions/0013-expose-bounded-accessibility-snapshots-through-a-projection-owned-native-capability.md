# ADR: Expose bounded accessibility snapshots through a projection-owned native capability

## Status
Accepted

## Date
2026-07-20

## Context

Windows UI Automation providers may be queried off the GLFW owner thread, while terminal, mux and projection mutation remain owner-thread or lock-owned. Exposing mutable terminal state directly would create races, stale identities, event floods and a privacy boundary. Multiple independent WndProc subclasses would make restore/chaining ownership ambiguous now that native IME owns a projection-local subclass host.

## Decision

1. `internal/accessibility` owns detached immutable documents, stable node/range identities, logical grapheme offsets, caret/selection, focus precedence, revisions and bounded semantic events. It performs no native calls and imports no GLFW, Win32, mux implementation or PTY package.
2. The frontend builds snapshots from detached render/core/mux/modal/search views. Text follows logical grapheme order; rectangles use rendered BiDi mappings. Alternate screen exposes only its active visible document.
3. Privacy scope is the active visible viewport plus focused input chrome. Scrollback, hyperlink targets, shell environment, command metadata, notification bodies and hidden/minimized workspace/window content are excluded.
4. Documents are bounded to 512 rows, 16,384 graphemes, 1 MiB UTF-8 and 256 nodes. Native ranges carry projection/document identity and fail stale; they never point into terminal storage.
5. Focus precedence is modal, search, then focused terminal pane. IME preedit is represented only through the active input snapshot.
6. Semantic changes coalesce by generation on the owner thread. Repaint-only damage emits nothing; native delivery is listener-gated; overflow collapses to document-invalidated.
7. Native providers are read-only. Off-thread calls read only atomically published immutable documents.
8. IME and accessibility register deterministic handlers in one bounded projection-owned Windows message router; no second WndProc subclass is installed.
9. Teardown stops publication, marks/disconnects providers, unregisters accessibility, then restores/releases the shared native host before unbind/resource/HWND destruction. Ambiguous cleanup retains ownership.
10. Windows activation is strict-v2, restart-scoped, visible-only, default-off and fail-closed per window. Unsupported/failure paths preserve terminal input/rendering.

## Consequences

- Core, VT, mux and PTY remain independent of UI Automation/COM.
- UIA reads are race-safe and cannot retain terminal buffers.
- Visible terminal text is exposed only after explicit opt-in; diagnostics never log it.
- Windows is the only adapter in this phase. macOS NSAccessibility and Linux AT-SPI require separate adapters.
- Default enablement and support claims remain blocked on real assistive-technology qualification.

## Rejected alternatives

- Query mutable terminal/App state directly from COM callbacks.
- Install a second WndProc subclass.
- Expose unlimited scrollback.
- Treat rendered visual order as text order.
- Implement mutable UIA actions in the initial release.

## Close-out evidence — 2026-07-20

- Phase 12.1–12.9 implementation merged through local dev `5794064`.
- Immutable focused-benchmark candidate: `1dfded5`; synchronized fingerprinted process-evidence harness: `e091a84`.
- Automated projection, privacy, scheduler, provider, COM ABI, lifecycle, race, process and allocation gates pass.
- Validation: `docs/validation/phase-12-accessibility-qualification.md` and `docs/validation/phase-12-accessibility-closeout.md`.
- Independent findings: `docs/validation/phase-12-review-disposition.md`.
- Narrator/NVDA/Inspect/UIA Verify rows remain explicit SKIP; the feature remains Windows-only, experimental and default-off.
