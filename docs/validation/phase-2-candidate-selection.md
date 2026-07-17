# Phase 2 Candidate Selection Validation

## Scope

This slice extends the candidate-only ADR-0002 pipeline with named environment/profile declaration and selection. It does **not** wire CLI flags, read process environment variables, publish staged Teal output, activate public composition, watch dependencies, install bundles, or create runtime scopes.

## Verified contracts

- `environments` and `profiles` are v2-only maps of strict partial ordinary configuration documents.
- Empty/non-string names, non-table values, unknown fields, nested composition/version/default metadata, malformed keys/actions/events, and unset selection defaults reject the candidate with source/path context.
- Same-name declarations apply in canonical graph post-order; unselected declarations do not affect the effective candidate.
- Ordinary include/primary fields apply first, the selected environment applies next, and the selected profile applies last.
- Environment selection is explicit override → supplied `CERVTERM_ENV` value → configured default → exact GOOS declaration; profile selection is explicit override → supplied `CERVTERM_PROFILE` value → configured default.
- Missing explicit, variable-supplied, or configured selections fail. A missing GOOS declaration silently selects no environment.
- The selection API is pure: callers supply pointer-presence inputs and the package performs no argument or process-environment reads.
- Configured-default choices retain their source origin. Effective selected fields retain environment/profile layer name, requested/canonical source, authored/migrated version, and overwrite chain.
- `cervterm.config.unset` works inside selected partial documents and a later selected layer may override a tombstone.
- Selected-layer operations participate in the composition node/list-entry limit; exact-boundary and repeated deterministic composition tests pass.
- Public unversioned/v1 and v2 loaders continue to reject all selection metadata and nested unset usage.
- Teal declarations model partial environment/profile documents without weakening the existing runtime setter types.

## Validation evidence

```text
go test ./... -count=1                                      PASS
go test -tags headless ./... -count=1                       PASS
go test -tags glfw ./... -count=1                           PASS
go test -race ./internal/config ./internal/script -count=1  PASS
go vet ./internal/config ./internal/script                  PASS
go test ./internal/script -run Teal -count=1                PASS
python -m json.tool docs/parity-support-matrix.json          PASS
git diff --check                                            PASS
```

Independent final blocker review reported **NO BLOCKERS**.

## Activation gate

Selection remains candidate-only. Public activation still requires transactional Teal publication and atomic ownership transfer of configuration, selected layers, Lua runtime, bindings/events, provenance, and dependency graph.
