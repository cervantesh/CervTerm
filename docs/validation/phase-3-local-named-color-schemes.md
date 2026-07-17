# Phase 3 Local Named Color Scheme Validation

## Scope

Implement ADR-0009 local `color_schemes` declarations and the composable `color_scheme` selector without changing terminal core, renderer backends, or runtime-patch precedence. A selected scheme is applied once before explicit `colors.*`; failed selection or validation preserves the active config and Lua runtime.

## Contracts

- `color_schemes` is a top-level v2 catalog. Includes and the primary source may compose duplicate names; environments and profiles can select a scheme but cannot declare a nested catalog.
- Every declared palette is strictly validated, including unselected entries. Schemes support foreground, background, cursor, selection background, exact ANSI 16, and sparse indexed colors 16–255.
- `color_scheme` composes through include, primary, environment, profile, and CLI precedence. An absent selector preserves the existing default palette.
- Effective palette precedence is defaults < selected scheme < explicit `colors.*` in normal source order < runtime patches.
- Scheme provenance is value-free and identifies the selected scheme plus its declaration source/version. Diagnostics expose the selector and effective color fields, not catalog contents.
- The public Teal contract provides `ColorScheme`, top-level `Config.color_schemes`, and composable `color_scheme`. `PartialConfig` intentionally does not expose a nested catalog.
- The typed example declares and selects the exact Shades of Purple fixture.
- GLFW reload evaluates an included selected scheme as one candidate: valid edits replace config/runtime atomically, while invalid edits retain the previous config/runtime.

## Evidence

- `internal/config/named_schemes_test.go`: strict validation, exact Shades of Purple values, declaration/selector precedence, duplicate merges, unsets, explicit-color precedence, compatibility, and provenance.
- `cmd/cervterm/config_diagnostics_test.go`: selector and effective scheme provenance are visible while catalog values remain absent.
- `internal/frontend/glfwgl/reload_test.go`: selected included scheme changes atomically; an invalid edit preserves prior config/runtime.
- `internal/config/template.go`: commented local scheme declaration and selector.
- `docs/examples/cervterm.d.tl` and `docs/examples/cervterm.tl`: public types and exact typed Shades of Purple selection.

## Verification commands

```text
gofmt -w internal/config/template.go cmd/cervterm/config_diagnostics_test.go internal/frontend/glfwgl/reload_test.go
go test ./internal/config ./internal/script -count=1
go test ./cmd/cervterm -count=1
go test ./internal/frontend/glfwgl -tags glfw -count=1
go test ./cmd/cervterm ./internal/frontend/glfwgl -tags glfw -count=1
tl check --include-dir docs/examples docs/examples/cervterm.tl
```

The GLFW-tagged commands require the platform GLFW/OpenGL development dependencies. Teal verification requires `tl` on `PATH`.
