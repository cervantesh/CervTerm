# cervterm MVP Architecture

## Decisions

- Language: Go for the MVP, with the explicit assumption that Go may not be the final best tool.
- UI/toolkit: no Fyne, no Gio, no widget toolkit. The MVP uses a thin GLFW/OpenGL frontend.
- Inspiration: Alacritty first (small, fast, layered), with boundaries that can grow toward WezTerm (mux, panes, domains).
- Performance policy: correctness and boundaries first; measure GC/allocation impact from day one; optimize only with evidence.
- Graphics backend: OpenGL through GLFW remains the only supported backend. Vulkan work is paused indefinitely; see [Rendering backend decision](rendering-backend-decision.md).

## Layering

```text
cmd/cervterm
  -> internal/frontend/glfwgl  window, input, OpenGL projection (optional tag)
  -> internal/mux             pane IDs, split tree, focus, layout, lifecycle and session aggregates
  -> internal/render          renderer-neutral per-pane frame snapshots
  -> internal/pty             local PTY/ConPTY byte transports
  -> internal/vt              escape parser, toolkit-neutral
  -> internal/core            per-pane grid, cells, cursor, attributes and scrollback
  -> internal/metrics         GC/allocation/frame counters
```

The core never imports the mux, renderer, PTY, GLFW, or OpenGL. Each mux pane owns one PTY, VT parser, terminal core and render snapshot. PTY readers enqueue pane-addressed bytes; the GLFW main thread serializes parsing, topology, focus, lifecycle and rendering. The frontend projects positioned panes and routes input; it does not own the split tree or sessions.

## Native in-process mux

```text
Frontend -> Mux Window -> implicit Tab -> SplitTree -> Pane -> local Session -> Terminal Core
```

The mux is process-local and supports native column/row splits, stable split identities and ratios, draggable dividers, focused-pane input, independent scrollback/selection/search/mouse/zoom state, deterministic close/collapse and clipped rendering. GLFW projects pointer and font intent, while `internal/mux` validates ratios and owns pixel/grid geometry using renderer-neutral metrics per pane. Terminal grids update live and PTY resize settles once after divider or pane-zoom interaction. Mixed font sizes share one bounded two-page glyph atlas whose entries are namespaced by raster specification; selecting a pane never clears atlas pages. Persistence, detach/reattach, visible tabs, remote domains and tmux integration remain deferred. IDs and pane-addressed commands/events avoid GLFW pointers so a future daemon can preserve the model without moving topology into the frontend.

## Verifiable measurements

Run parser/core allocation checks:

```bash
go test ./internal/vt -bench=. -benchmem
go test ./internal/render -bench=. -benchmem
```

Run runtime GC tracing:

```bash
GODEBUG=gctrace=1 go run ./cmd/cervterm
```

MVP overlay shows bytes read, frames, malloc count, heap, GC count, and last GC pause. This is intentionally visible so GC/reuse discussions stay evidence-based.
