# Phase 3 Semantic Chrome Color Validation

## Scope

Complete Phase 3 slice 4 by adding six live semantic color tokens to inline `colors` and local `color_schemes`, then route application chrome through those tokens without changing terminal cell color semantics or renderer backends.

## Contracts

- `chrome_background`, `chrome_muted`, `accent`, `split`, `search_match`, and `error` accept `#RRGGBB` or `#RRGGBBAA` and default to the colors used before this slice.
- The fields participate in strict v2 validation, composition, value-free provenance, selected named schemes, live diffing, and atomic reload.
- Effective precedence remains defaults < selected scheme < explicit `colors.*` < runtime patches.
- `chrome_background` and `accent` color status, search, and HUD surfaces; `chrome_muted` colors secondary chrome text.
- `split` colors pane dividers, `error` colors failed/exited focused-pane state, and `search_match` colors terminal search hits.
- Existing `scrollbar.*_color` fields continue to own scrollbar track and thumb colors.
- The public Teal contract exposes all six fields in `ColorsConfig`, `ColorsDocument`, and `ColorScheme`; typed schemes may specify only a subset.
- Invalid reload candidates preserve the previous config, Lua runtime, and rendered chrome colors.

## Evidence

- `internal/config/config_test.go`, `document_test.go`, `document_validate.go`, `named_schemes_test.go`, and diff/diagnostic tests cover defaults, strict validation, composition, provenance, named schemes, live classification, and diagnostics.
- `internal/frontend/glfwgl/chrome_colors_test.go` covers exact default and custom RGBA resolution.
- `internal/frontend/glfwgl/draw_list_test.go`, `app_row.go`, `app_draw.go`, and `app_status.go` cover semantic routing to HUD, search/status surfaces, pane chrome, and search matches.
- `internal/frontend/glfwgl/reload_test.go` covers atomic named-scheme semantic-color reload and invalid-candidate preservation.
- `internal/config/template.go` publishes the six current defaults and documents the fields in a reusable scheme.
- `docs/examples/cervterm.d.tl` and `docs/examples/cervterm.tl` expose the public types and demonstrate a typed scheme that sets a semantic subset.

## Verification commands

```text
gofmt -w internal/config/template.go
go test ./... -count=1
go test ./internal/frontend/glfwgl -tags glfw -count=1
tl check --include-dir docs/examples docs/examples/cervterm.tl
```

All commands passed on the slice worktree. Teal verification requires `tl` on `PATH`.
