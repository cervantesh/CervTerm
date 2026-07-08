# cervterm MVP Architecture

## Decisions

- Language: Go for the MVP, with the explicit assumption that Go may not be the final best tool.
- UI/toolkit: no Fyne, no Gio, no widget toolkit. The MVP uses a thin GLFW/OpenGL frontend.
- Inspiration: Alacritty first (small, fast, layered), with boundaries that can grow toward WezTerm (mux, panes, domains).
- Performance policy: correctness and boundaries first; measure GC/allocation impact from day one; optimize only with evidence.

## Layering

```text
cmd/cervterm
  -> internal/frontend/glfwgl  window, input, OpenGL drawing (optional tag)
  -> internal/render           renderer-neutral frame snapshots
  -> internal/pty              local PTY now, ConPTY/SSH/serial later
  -> internal/vt               escape parser, toolkit-neutral
  -> internal/core             grid, cells, cursor, attributes
  -> internal/metrics          GC/allocation/frame counters
```

The core never imports the renderer, PTY, GLFW, or OpenGL. The render snapshot copies core cells into a stable frame. Frontends consume snapshots; they do not parse ANSI. The PTY only moves bytes.

## Future WezTerm-like growth

Add `internal/mux` later:

```text
Frontend -> Mux Window -> Tab -> Pane -> Domain(local/ssh/serial) -> Terminal Core
```

This allows tabs, splits, SSH domains, and remote multiplexing without rewriting the core terminal state.

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
