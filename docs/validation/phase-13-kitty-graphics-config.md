# Phase 13.8 — Dormant Kitty graphics configuration

Date: 2026-07-21
Branch: `feat/parity-phase-13-image-config`
Base: local `dev` at `80e435b`

## Contract

The approved v2 schema is now public and restart-scoped:

- `graphics.kitty.enabled` (default `false`)
- `graphics.limits.encoded_bytes_per_pane`
- `graphics.limits.decoded_bytes_per_pane`
- `graphics.limits.image_count_per_pane`
- `graphics.limits.placement_count_per_pane`
- `graphics.limits.gpu_bytes_per_context`

The explicit values shown in the approved design are the accepted hard-cap defaults. Values may be lowered but cannot be zero or exceed a cap. The fixed 512-entry GPU cache bound is not configurable in this schema.

No field is passed to mux, parser, decoder, renderer, scheduler, or GPU code in this slice. `enabled = true` remains dormant intent and creates no capability or side effect.

## Coverage

- Strict v2 decode and explicit v1 rejection.
- Default-off hard-cap values and accepted approved default document.
- Zero/raised limit rejection with field-qualified candidate diagnostics.
- Includes, profile selection, leaf unset, and winning-source provenance.
- Schema metadata, CLI-compatible generic composition, template output, and public Teal declarations.
- Restart-only `DiffConfig`; `MergeLiveConfig` preserves all effective graphics values.
- Doctor output reports configured intent, limits, and dormant activation.
- Generated Teal default example type-checks and loads.

## Verification

- `go test ./... -count=1` — pass.
- `go test -race ./internal/config ./internal/script -count=1` — pass.
- `go vet ./internal/config ./internal/script ./cmd/cervterm` — pass.
- `go run ./scripts/check-phase13-imports.go` — pass.
- `go test ./internal/script -run 'TestTypedDefaultTealExample|TestTealConfigUnsetTypeContract' -v -count=1` — pass with `tl` installed.

## Deferred

Mux/frontend activation, protocol advertisement, Kitty APC decoding, image decode scheduling, GPU caches, and presentation remain deferred to subsequent Phase 13 slices.
