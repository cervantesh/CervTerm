# Phase 2 Candidate CLI Override Validation

## Scope

This slice adds a pure candidate-only typed CLI override layer. It does **not** expose an executable flag, read process arguments, create runtime scopes, publish Teal output, activate composition, install bundles, or watch dependencies.

## Verified contracts

- Overrides apply left-to-right after ordinary, environment, and profile layers.
- Paths resolve against schema metadata; scalar and list capability flags are exposed through `SchemaFields`.
- JSON booleans/numbers/integers/string arrays decode strictly; schema-known strings may be quoted JSON or unquoted text.
- Nulls, malformed/trailing JSON, mixed arrays, fractional/out-of-range integers, and integers that would lose float64 precision reject the candidate.
- Records, maps, bindings, callbacks, and composition metadata are not CLI-overridable.
- `shell.env` and subkeys reject before raw-value decoding because process argument lists are observable.
- Errors and provenance never retain or print raw override values.
- Repeated paths keep a low-to-high CLI provenance chain with immutable argument-index metadata; callers cannot mutate stored audit records.
- CLI list entries participate in the existing composition node/list-entry limit.
- Final cross-field/range validation remains the future candidate-bundle caller's responsibility.

## Validation evidence

```text
go test ./... -count=1                                      PASS
go test -tags headless ./... -count=1                       PASS
go test -tags glfw ./... -count=1                           PASS
go test -race ./internal/config ./internal/script -count=1  PASS
go vet ./internal/config ./internal/script                  PASS
```

Independent review found and verified fixes for large-integer precision loss and mutable CLI-index provenance. Final blocker-only review reported **NO BLOCKERS**.

## Activation gate

No public command-line flag exists. Wiring remains gated on transactional Teal publication and atomic bundle ownership so a failed override or final validation cannot partially replace the active runtime.
