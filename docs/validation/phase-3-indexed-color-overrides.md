# Phase 3 Indexed Color Override Validation

## Scope

Add live sparse configuration for xterm palette indices 16–255 on top of logical renderer-time color resolution. Preserve ANSI ownership of 0–15, algorithmic cube/grayscale fallback, truecolor, atomic reload, and the explicit renderer-backend exclusion.

## Contracts

- `IndexedColorOverrides` is a comparable fixed `[240]string` value; slot zero represents index 16 and empty entries mean fallback. No mutable map aliasing or clone work is introduced.
- Lua/Teal use numeric keys: `indexed_colors = { [16] = "#102030", [196] = "#FF1010" }`.
- Strict validation rejects indices below 16 or above 255, negative/fractional/string keys, non-string values, malformed colors, and alpha. ANSI 0–15 can only be configured through `colors.ansi`.
- V2 composition merges keys numerically, charges one node per entry, and records value-free provenance paths such as `colors.indexed_colors[196]`. Per-key unset removes one override; whole-field unset removes all prior dynamic keys. No 240 fabricated default provenance winners exist.
- Missing or unset entries resolve through the unchanged xterm cube/grayscale algorithm.
- The field is live, comparable, and diffed as one configuration leaf. Existing full-Colors live assignment and damage invalidation provide atomic commit/rollback and existing-cell reprojection.
- CLI and runtime overrides are intentionally unavailable; schema metadata and value-safe rejection agree.
- Diagnostics JSON-marshal only configured slots as a deterministic sparse object and include per-key provenance beneath the leaf.
- `ColorResolver` materializes a private dense 256-entry table once per frame. Sparse overrides can modify only 16–255 through `SetIndexed`; row rendering receives the resolver by pointer and performs O(1) lookups.
- ANSI and truecolor remain invariant when sparse indexed overrides change.
- OSC palette mutation/query/reset, named schemes, semantic chrome, and selectable renderer backends remain deferred.

## Test evidence

- `internal/config/indexed_colors_test.go`: strict keys/values, sparse decode, fallback, numeric composition, per-key/whole unset, provenance, sparse diagnostics, metadata, comparability, and live diff.
- `internal/config/cli_override_test.go`: whole-map CLI rejection without raw value exposure.
- `internal/core/color_test.go`: sparse override, ANSI protection, neighbor fallback, and all 256 default resolver values against the legacy xterm function.
- `internal/frontend/glfwgl/palette_resolver_test.go`: the same logical index reprojects under two sparse configurations while neighboring fallback and ANSI remain stable.
- Existing reload tests cover full Colors commit, damage invalidation, and failed-candidate preservation.
- Teal declarations type the sparse map as integer-to-string/Unset; runtime validation remains authoritative for the range.

## Verification commands

```text
go test ./... -count=1
go test -tags headless ./... -count=1
go test -tags glfw ./... -count=1
go test -race -tags glfw ./internal/config ./internal/core ./internal/frontend/glfwgl -count=1
go vet ./internal/config ./internal/core
go vet -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm
go run ./scripts/check-maturity-gates.go
python -m json.tool docs/parity-support-matrix.json
```
