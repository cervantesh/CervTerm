# Phase 12 architecture drift check

Date: 2026-07-20
Activation merge: `5794064`
Close-out diagnostic/test chain: `9d7b813` → `cd990bc` → `a2ca359` → `1dfded5` → `db7c312` → `e091a84`
Result: **PASS — no unapproved architecture drift**

- `internal/accessibility` remains toolkit-neutral; GLFW/UIA dependencies remain in `internal/frontend/glfwgl`.
- UIA, WndProc, GLFW window and native event operations remain projection-owner-thread frontend concerns.
- Mux remains authoritative for window/tab/pane/workspace identity, topology, focus and geometry.
- Accessibility consumes detached immutable render/core/mux/modal values and never exposes live terminal pointers.
- IME and accessibility reuse one bounded transactional WndProc router; no second subclass owner was introduced.
- The capability remains strict-v2, restart-scoped, visible-only, Windows-only, default-off and fail-closed per window.
- Hidden/minimized/inactive content, off-viewport scrollback, URIs, process/environment metadata, handles, tokens and notification bodies remain excluded.
- Semantic changes remain generation-checked/coalesced; repaint-only events remain suppressed; native events remain listener-gated.
- Close-out additions are tests, documentation and an `accessibilitymetrics`-only probe. Normal builds compile a no-op probe; no production persistence, migration, infrastructure, renderer selection, external domain or policy direction changed.
- The machine-readable support matrix says `status=experimental`, `default_enabled=false`, and `support_claim=none`.

Accepted ADR: `.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0013-expose-bounded-accessibility-snapshots-through-a-projection-owned-native-capability.md`.
