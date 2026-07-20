# Phase 12.3 — Accessibility tree and focus composition

## Scope

Adds pure, bounded composition of one visible native-window projection into window, active-tab, visible-pane terminal and focused-input nodes. Adds dormant GLFW value adapters for detached mux/render snapshots plus modal, search and IME preedit state. No publication, scheduler, provider, native API or runtime activation is introduced.

## Contracts

- A document belongs to exactly one provider generation and native-window projection. A hidden workspace/window returns no document before any pane value is inspected.
- Only the active tab is traversed. Inactive tab titles and terminal snapshots never enter the composer; active pane order follows detached mux `TabView.Panes` order.
- Node identities use projection plus stable mux object IDs. Pane lifecycle activations are explicit capture inputs. Modal/search IDs retain activation across query revisions; IME preedit uses composition generation in a reserved input-surface namespace.
- Focus precedence is modal, then search, then focused pane. Matching preedit is a child of that focus surface and never steals focus. Stale preedit parent/activation is omitted atomically.
- Window, tab and pane labels at the frontend edge are explicit safe labels. OSC/process titles, CWD, hyperlink URIs, hidden-workspace values and modal entry details are not copied.
- Terminal content uses the Phase 12.2 projector with global remaining row/grapheme/byte budgets. Node count is capped before further panes are projected. Truncation may omit an out-of-budget focus target but cannot create a stale node.
- The pure package remains independent of core, render, mux, modal, IME, frontend, PTY and native APIs. Frontend adapters clone pointer values and detach render cells before composition.

## Evidence

Deterministic goldens cover active-tab/pane privacy and modal-over-search/preedit precedence. Focused tests cover hidden-workspace short-circuiting without pane capture, safe-title isolation, multi-window isolation, pane close/cross-window move identity, query revision versus activation, modal/search/pane focus transitions, matching and stale IME preedit, grapheme caret/target spans, ambiguous active tabs, missing/stale focus, global 256-node and 512-row bounds, snapshot detachment and terminal projection compatibility.

## Gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go run ./scripts/check-maturity-gates.go
git diff --check
```
