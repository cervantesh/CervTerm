# Phase 3 OSC Palette Validation

## Scope

Complete Phase 3 with ADR-0010 bounded, ephemeral, pane-local dynamic palette behavior. The implementation adds OSC 4/10/11 set/query and OSC 104/110/111 reset without moving mutable palette state into config, mux-global presentation state, or renderer storage.

## Contracts

- Each terminal/pane owns fixed-size overrides for indexed slots 0–255 and default foreground/background.
- Effective precedence is configured palette < pane-local OSC override < explicit cell truecolor.
- Successful config reload replaces the base palette beneath retained pane overrides; reset reveals the new base.
- OSC 4 accepts at most 256 complete index/spec pairs. OSC 104 accepts at most 256 indexes. Commands validate completely before mutation or reply.
- Accepted set forms are `#RRGGBB` and `rgb:H/H/H` with one to four hexadecimal digits per component.
- Queries emit canonical uppercase `rgb:RRRR/GGGG/BBBB` replies through the originating pane's existing bounded PTY reply path.
- OSC 104/110/111 accept standard delimiter-free resets; malformed, unsupported, incomplete, excessive, or overlong payloads are silent and atomic.
- OSC 11 changes RGB while preserving configured background alpha/transparency.
- Dynamic overrides are not persisted or included in config provenance and disappear with the pane.

## Evidence

- `internal/core/palette_test.go`: fixed-state mutation/reset, generation, base replacement, resolver precedence, and truecolor invariance.
- `internal/vt/parser_osc_palette_test.go`: BEL/ST/chunking, set/query/reset, canonical replies, color scaling, malformed atomicity, and 256/257 boundaries.
- `internal/render/snapshot.go`: fixed palette override values are captured with each pane snapshot.
- `internal/mux/mux_test.go`: configured base propagation, effective queries, new-pane inheritance, sibling isolation, and reload beneath an override.
- `internal/frontend/glfwgl/palette_resolver_test.go`: configured reload beneath retained OSC overrides, reset behavior, truecolor invariance, and background-alpha preservation.
- `internal/frontend/glfwgl/app_draw.go`: per-pane resolver/default colors and pane background projection without scrollback reparsing.

## Verification commands

```text
gofmt -w internal/core/palette.go internal/vt/parser_osc.go internal/mux/mux.go internal/frontend/glfwgl/palette_resolver.go internal/frontend/glfwgl/app_draw.go
go test ./internal/core ./internal/vt ./internal/render ./internal/mux -count=1
go test ./internal/frontend/glfwgl -tags glfw -count=1
go test ./... -count=1
```

The GLFW-tagged command requires platform GLFW/OpenGL development dependencies.
