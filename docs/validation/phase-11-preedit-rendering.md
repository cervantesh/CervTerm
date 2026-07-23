# Phase 11.4 Validation — Projection-Local Preedit Rendering

## Contract

- Successful composition start/update/commit/cancel publishes a detached revision snapshot and requests an on-demand frame only for the owning projection. Stale or rejected events publish nothing and schedule no frame.
- Preedit remains frontend-only. Terminal presentation is drawn in the active pane font/zoom context, inside its renderer clip, after the terminal cursor and script overlays. Search and retained-modal presentation is drawn at the end of their input query and clipped to the owning chrome panel.
- Presentation segments extended grapheme clusters, bounds visual work to 256 cells, truncates only at cluster boundaries, and maps logical caret/target indices into visual cell positions. BiDi runs are reordered without splitting clusters; contiguous RTL clusters are shaped as one run for cursive context; mixed-direction target clauses retain disjoint visual spans rather than highlighting intervening text.
- Target clauses use translucent selection, the full preedit uses an underline, and the caret is kept inside the reserved clip even when text exactly fills available cells. Cluster fallback preserves combining placement and never draws beyond the cluster cell budget.
- The current modal/search/pane activation is revalidated before drawing, so stale composition state cannot appear on a replacement target.
- Composition active/revision/target participates in double-buffer damage. Each mutation repaints both back buffers, then returns to incremental idle; no timer or periodic IME cadence is introduced.
- No native IME API, candidate geometry, configuration, persistence, mux/core cell mutation, or PTY preedit I/O is added.

## Evidence

Tests cover CJK, combining text, emoji ZWJ clusters, RTL visual order, mixed-direction disjoint target spans, caret and clause mapping, cluster-safe clipping, the 256-cell budget, inactive/zero geometry, trailing-caret containment, mutation-only presentation notifications, owning-projection redraw isolation, and two-buffer damage returning to idle. Existing Phase 11.3 tests continue to prove zero PTY bytes before commit.

## Required gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race ./... -count=1
go run ./scripts/check-maturity-gates.go
git diff --check
```

## Rollback

Remove the preedit presentation/draw hooks and composition damage fields, then remove the mutation presentation callback. The Phase 11.1 model, 11.2 committed-text router and 11.3 lifecycle coordinator remain valid and continue to keep preedit out of PTY/core/mux state.
