# Phase 2 Executable Config Input Validation

## Scope

Wire ADR-0002 named environment/profile selection and repeatable typed CLI overrides from the installed executable into the already-validated v2 composition pipeline. Preserve an immutable startup snapshot across reload. Doctor/explain rendering and native-window override routing remain deferred.

## Public inputs

- `--environment NAME` and `CERVTERM_ENV`
- `--profile NAME` and `CERVTERM_PROFILE`
- repeatable `--config-override PATH=VALUE`

Flags preserve presence separately from value, including explicit empty strings. Environment variables use lookup/presence semantics. Selection remains flag, environment variable, configured default, then exact GOOS fallback for environments; profile omits GOOS.

Each override splits at the first `=`, retains left-to-right order, and records the one-based command-line flag token index in provenance. Values continue through the schema-owned decoder already covered by `phase-2-candidate-cli-overrides.md`; sensitive and unsupported fields reject without rendering the raw value.

## Ownership and reload

The executable builds one detached `script.CandidateOptions` snapshot. V2 candidate bundles retain a deep copy of selection pointers and ordered override slices. Startup transfers the bundle options into `App`; every reload passes a fresh detached clone to `LoadVersioned`, so ambient environment changes or caller mutation cannot change the active process selection.

Explicit composition flags require a source authored with `config_version = 2`. They reject no-source startup, authored-v1 startup, and v2-to-v1 reload. Ambient `CERVTERM_ENV`/`CERVTERM_PROFILE` remain ignored on v1 to preserve the compatibility path. A rejected transition closes candidate resources, rolls back any legacy transition, and keeps the previous bundle/config active.

## Test evidence

- `cmd/cervterm/config_flags_test.go`: absent versus present-empty values, environment lookup presence, both Go flag forms, first-`=` splitting, left-to-right order, stable argument indexes, malformed input redaction, `--` handling, and v1/no-source boundaries.
- `internal/script/candidate_bundle_test.go`: deep option cloning and explicit-v2 requirement classification.
- `internal/frontend/glfwgl/reload_test.go`: startup selection/CLI winners survive reload with unchanged selection and CLI provenance; caller mutation is detached; a v1-started process retains ambient selection for a later v2 source; explicit v2-to-v1 rejection preserves active ownership.
- Existing selection and CLI decoder suites continue to cover precedence, missing names, GOOS fallback, schema capabilities, value coercion, validation, node limits, sensitive paths, and overwrite chains.

## Verification commands

```text
go test ./... -count=1
go test -tags headless ./... -count=1
go test -tags glfw ./... -count=1
go test -race -tags glfw ./internal/config ./internal/script ./internal/frontend/glfwgl ./cmd/cervterm -count=1
go vet ./internal/config ./internal/script ./cmd/cervterm
go vet -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm
go run ./scripts/check-maturity-gates.go
python -m json.tool docs/parity-support-matrix.json
```

## Deferred

- `--explain-config`, field filtering, and doctor selection/provenance rendering.
- Failed-attempt dependency-union recovery.
- Per-native-window origin/focus mapping after ADR-0004.
