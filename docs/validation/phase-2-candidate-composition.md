# Phase 2 Candidate Composition Validation

## Scope

This slice implements the candidate-only ADR-0002 merge/provenance seam. It does **not** activate public includes, publish staged Teal output, select profiles/environments, install a runtime bundle, watch dependencies, or apply overrides.

## Verified contracts

- Canonical source-graph post-order is consumed once, low to high, in the graph's owning Lua state.
- Records merge recursively; `shell.env` merges by key; lists replace; event callbacks merge by function slot.
- Unversioned/v1 includes contribute only compatible fields they supplied and retain authored/migrated schema versions in provenance.
- `cervterm.config.unset` is an unforgeable userdata tombstone accepted only by composed v2 candidates. Whole records/maps/lists/events, leaves, map keys, and event slots reset lower layers; higher layers can win later.
- Public unversioned, explicit-v1, and v2 single-source loaders reject `unset`; public loaders continue to reject `includes`.
- Provenance stores built-in/include/primary origins, requested/canonical source, authored/migrated versions, low-to-high overwrite chains, tombstone state, and sensitivity without storing raw values.
- Absent maps, bindings, and callbacks do not receive fabricated default provenance.
- Composition enforces state ownership and a bounded node/list-entry count, including exact-boundary and deterministic-repeat tests.
- Lua and Teal contracts expose `cervterm.config.unset`; Teal unions preserve existing runtime config types and compile under the repository's `tl` tests.

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

Independent review found the initial public-v1 unset gap and fabricated absent defaults; both were corrected. Final blocker-only re-review reported **NO BLOCKERS**.

## Activation gate

The public `includes` and `unset` surfaces remain fail-closed. Activation requires transactional Teal publication and atomic ownership transfer of configuration, Lua runtime, bindings/events, provenance, and dependency graph so a failed candidate cannot partially replace the last-known-good runtime.
