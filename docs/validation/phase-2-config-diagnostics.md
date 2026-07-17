# Phase 2 Config Diagnostics Validation

## Scope

Expose ADR-0002 composed configuration evidence through read-only `--explain-config`, repeatable `--explain-config-field`, and `--doctor`. Diagnostic commands must exit before logging setup, profiling, GLFW initialization, window/frontend creation, PTY spawn, candidate activation, or Teal publication.

## Contracts

- Both headless and GLFW entrypoints register identical explanation flags, snapshot environment/profile/CLI inputs after informational exits, and route explain/doctor before runtime startup.
- Explanation requires an authored v2 source. No source, v1, or unknown/non-leaf field filters return usage status 2; evaluation/validation errors return status 1.
- Empty filters explain every schema-v2 public leaf. Repeated exact filters deduplicate while preserving schema order.
- Resolved scalar/list values use deterministic JSON syntax. `shell.env` and any schema-sensitive field always render `<redacted>`; map contents never reach the diagnostic model. Lua functions/key/event bodies, source bytes, hashes, raw CLI values, and Teal staging paths are absent.
- Runtime-owned keys/events render only `<configured>` or `<unset>` from value-free provenance.
- Provenance is detached and reports winner plus low-to-high overwritten origins, selection basis, source identity/version, CLI argument index, and runtime scope where applicable.
- Source graph diagnostics deep-copy source identities, aliases, migration versions, include edges, and local dependency identities while excluding content/hash/executable state.
- `CandidateOptions.DiagnosticOnly` suppresses legacy v1 Teal adjacent publication. V2 candidates are inspected and closed without activation/publication.
- Doctor uses the same loader. V2 prints composed selection/graph/fields; v1 and no-source report explicit compatibility boundaries. Pending changes and last reload failure are unavailable in a separate one-shot process. Invalid configuration makes doctor return status 1.
- GLFW doctor reports content scale as not probed rather than calling `glfw.Init`.

## Test evidence

- `internal/config/diagnostic_test.go`: deterministic schema order, filters, unknown paths, JSON values, sensitive redaction, runtime-owned markers, and detached provenance.
- `internal/config/source_graph_diagnostic_test.go`: graph snapshot completeness and deep detachment.
- `internal/script/candidate_diagnostic_test.go`: candidate diagnostic access and ownership detachment.
- `internal/script/versioned_load_test.go`: diagnostic-only v1 Teal evaluation does not publish adjacent Lua.
- `cmd/cervterm/config_flags_test.go`: explain flag presence and repeatable order.
- `cmd/cervterm/config_diagnostics_test.go`: composed profile/CLI explanation, field filtering/deduplication, sensitive-value absence, provenance, v1/no-source/unknown boundaries, and cleanup paths.
- `cmd/cervterm/doctor_test.go`: v1 compatibility, composed v2 output, sensitive redaction, inactive-process boundaries, content-scale non-probe, and nonzero invalid-config status.

## Verification commands

```text
go test ./... -count=1
go test -tags headless ./... -count=1
go test -tags glfw ./... -count=1
go test -race -tags glfw ./internal/config ./internal/script ./cmd/cervterm -count=1
go vet ./internal/config ./internal/script ./cmd/cervterm
go vet -tags glfw ./cmd/cervterm
go run ./scripts/check-maturity-gates.go
python -m json.tool docs/parity-support-matrix.json
```

Independent adversarial review reported **NO BLOCKERS**. Doctor load-failure exit status was strengthened from a review concern.

## Deferred

Native multi-window scope routing and Phase 2 closeout remain later slices.
