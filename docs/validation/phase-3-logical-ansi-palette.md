# Phase 3 Logical ANSI Palette Validation

## Scope

Introduce renderer-time palette resolution and a live configurable ANSI 16 while preserving the existing default appearance, xterm indices 16–255, truecolor, mux/snapshot behavior, and renderer-backend exclusion.

## Contracts

- `core.LogicalColor` is compact and comparable with distinct Default, Indexed, and RGB kinds. Literal RGB values equal to legacy default sentinel RGBs remain explicit.
- Terminal initialization, blanks, reset, SGR 0/39/49 use logical Default. Basic/bright SGR and `38/48;5;n` retain Indexed identity. `38/48;2` retains literal RGB.
- `ColorResolver` resolves foreground/background defaults independently, configurable ANSI indices 0–15, and the unchanged xterm cube/grayscale formula for 16–255.
- Snapshots and row damage hash logical attributes. They do not bake a palette or require terminal reparsing.
- `colors.ansi` is a live schema leaf: a dense list of exactly 16 `#RRGGBB` values in black/red/green/yellow/blue/purple/cyan/white then bright order. Alpha, sparse/wrong-length lists, and malformed colors reject.
- Omission supplies the previous hardcoded ANSI table exactly. Existing v1/v2 configurations retain identical output.
- Config schema, strict validation, Lua decode, CLI list coercion, desired/effective diff, atomic live merge, template, Teal declarations/examples, scripting docs, and explain diagnostics include the field.
- GLFW builds one resolver per frame from configured foreground/background/ANSI. Existing logical cells resolve differently after a successful live palette reload; truecolor remains invariant. Existing live commit damage invalidation forces repaint. Failed reload retains the old configuration.
- Inverse swaps resolved foreground/background before bold/dim as before. Bold/dim algorithms remain unchanged in this slice.
- Indexed overrides above 15, named schemes, semantic chrome routing, and OSC palette mutation remain later Phase 3 slices.

## Test evidence

- `internal/core/color_test.go` and `attr_test.go`: tagged constructors/accessors, default-vs-literal identity, configurable ANSI resolution, and xterm anchors.
- `internal/vt/parser_test.go`: logical basic/bright/indexed/truecolor/reset semantics and malformed sequences.
- `internal/render/snapshot_test.go` and damage tests: logical attribute copying/hashing.
- `internal/config` schema/config/Lua/template/diff/diagnostic tests: exact list contract, default compatibility, live scope, and rendering in diagnostics.
- `internal/frontend/glfwgl/palette_resolver_test.go`: one unchanged logical cell reprojects under two palettes while truecolor remains stable; configured logical defaults resolve correctly.
- Existing frontend reload tests retain atomic color commit/rollback and full damage invalidation coverage.

## Verification commands

```text
go test ./... -count=1
go test -tags headless ./... -count=1
go test -tags glfw ./... -count=1
go test -race -tags glfw ./internal/core ./internal/vt ./internal/render ./internal/config ./internal/frontend/glfwgl -count=1
go vet ./internal/core ./internal/vt ./internal/render ./internal/config
go vet -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm
go run ./scripts/check-maturity-gates.go
python -m json.tool docs/parity-support-matrix.json
```

## Deferred

- Sparse/configurable indexed colors 16–255.
- OSC 4/10/11/104/110/111 per-pane mutations, queries, and resets.
- Named schemes and semantic chrome tokens.
- Any selectable rendering backend.
