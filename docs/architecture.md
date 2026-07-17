# CervTerm Architecture

## Decisions

- Language: Go for the MVP, with the explicit assumption that Go may not be the final best tool.
- UI/toolkit: no Fyne, no Gio, no widget toolkit. The MVP uses a thin GLFW/OpenGL frontend.
- Inspiration: Alacritty first (small, fast, layered), with selected WezTerm-inspired local mux and UX capabilities. WezTerm-style domains are explicitly out of scope.
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
Frontend -> Mux Window -> implicit Tab -> SplitTree -> Pane -> local PTY Session -> Terminal Core
```

The mux is process-local and supports native column/row splits, stable split identities and ratios, draggable dividers, focused-pane input, independent scrollback/selection/search/mouse/zoom state, deterministic close/collapse and clipped rendering. GLFW projects pointer and font intent, while `internal/mux` validates ratios and owns pixel/grid geometry using renderer-neutral metrics per pane. Terminal grids update live and PTY resize settles once after divider or pane-zoom interaction. Mixed font sizes share one bounded two-page glyph atlas whose entries are namespaced by raster specification; selecting a pane never clears atlas pages. Visible tabs, multiple local windows, and layout-only workspaces are planned above these ownership boundaries. Domains, a daemon, live detach/reattach, remote sessions, and tmux integration are excluded.

## Candidate configuration source graph

`internal/config.BuildSourceGraph` is the candidate-only foundation for Phase 2 composition. It consumes one fresh caller-owned candidate Lua state, canonicalizes local Lua/Teal source identity (including filesystem aliases), evaluates one primary plus declarative includes exactly once in that state, and emits deterministic depth-first post-order nodes and dependency edges. A failed candidate state is discarded, and a state cannot be submitted for a second build. Depth, source-count, per-file byte, and aggregate-byte limits reject a candidate before it can affect active configuration.

Primary evaluation occurs once before include traversal so it can declare edges. Includes and their nested `require`/`dofile`/`loadfile` calls run under a declarative guard: they may return values and typed actions but cannot register timers, status entries, or overlays. The instrumented standard loaders record canonical local module dependencies and v2 rejects replacement/custom loaders that would make reload completeness unknowable.

Teal sources check and generate into a per-candidate owned staging directory (including beneath a caller-supplied staging parent); the graph reserves their eventual adjacent Lua paths and rejects source/derived-output collisions without publishing files. Candidate staging is removed when the graph closes.

`internal/config.ComposeSourceGraph` consumes that graph in deterministic post-order and builds a new root table in the same Lua state. Records merge recursively, `shell.env` merges by key, lists replace, event function slots replace independently, and `cervterm.config.unset` suppresses lower layers while allowing a higher value to win later. A 100,000-node/list-entry ceiling bounds composition. Provenance is retained per fixed schema leaf, map key, list, and event function with source versions and a low-to-high overwrite chain; it stores no raw values and marks sensitive paths.

Named `environments` and `profiles` are strict partial documents. Same-name declarations remain source-local and apply in graph order after ordinary include/primary fields, so the chosen environment wins over the base and the chosen profile wins over the environment without losing per-source provenance. The pure candidate selector resolves environment override, `CERVTERM_ENV` input, configured default, then exact GOOS fallback; profile resolution uses override, `CERVTERM_PROFILE` input, then configured default. Missing explicit/configured selections fail, while an absent GOOS match selects nothing.

This remains a candidate-only seam. Public loaders reject `includes`, selection metadata, and `unset` until Teal output publication and config/runtime/graph ownership can transfer as one atomic bundle; candidate composition does not mutate active configuration or publish generated files.

## Verifiable measurements

Run parser/core allocation checks:

```bash
go test ./internal/vt -bench=. -benchmem
go test ./internal/render -bench=. -benchmem
```
go run ./scripts/capture-parity-baseline.go -count 3

Run runtime GC tracing:

```bash
GODEBUG=gctrace=1 go run ./cmd/cervterm
```

MVP overlay shows bytes read, frames, malloc count, heap, GC count, and last GC pause. This is intentionally visible so GC/reuse discussions stay evidence-based.

The current cross-subsystem delivery contract is [`docs/wezterm-parity-roadmap.md`](wezterm-parity-roadmap.md); reproducible measurements are recorded in [`docs/parity-baseline.md`](parity-baseline.md).
