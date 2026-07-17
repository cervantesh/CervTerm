# Phase 2 Failed Reload Recovery Validation

## Scope

Complete ADR-0002 dependency-aware recovery after candidate failure. A broken edit must preserve the active bundle while watching both the last successful graph and the paths discovered by the latest failed attempt, including missing local sources. Repeated identical notices are bounded independently from retry eligibility.

## Contracts

- `SourceGraphFailureError` and `VersionedLoadError` preserve the original error text and `Unwrap` chain while carrying detached, sorted, value-free absolute path expectations.
- Graph traversal records selected paths before canonicalization, so missing declarative Lua/Teal includes remain watchable. Existing visited/canonical paths and local `require`/`dofile`/`loadfile` attempts are retained on failure.
- Failures after graph construction—scripting, composition, selection, typed overrides, final config validation, bindings/events, and Teal transitions—carry the successfully evaluated graph paths before candidate ownership closes.
- The frontend watcher stores successful and failure-only sets separately. Failure installs `successful ∪ latest failure`, replacing older failure-only paths. Success replaces the active set and clears failure-only paths.
- A newly discovered failure set queues one immediate attempt so a dependency repaired during evaluation cannot be acknowledged away. Thereafter normal 250 ms polling and whole-graph stable debounce trigger recovery.
- Watcher state is the only failure-side mutation. Config, desired/effective state, runtime scopes, Lua runtime, candidate bundle, Teal publication, mux, and UI resources remain last-known-good.
- Identical reload failure logs/UI notices are emitted at most once per 30 seconds. Different failures report immediately; success resets the gate. Notice suppression never clears `reloadPending`, changes watcher polling, or removes retry paths.
- Runtime-callback-only modules remain outside the evaluated configuration graph by design.

## Test evidence

- `internal/config/source_graph_test.go`: missing nested includes, missing local loader dependencies, existing Teal failure sources, detached evidence, and staging/state cleanup.
- `internal/script/versioned_load_test.go`: graph failure evidence and successful-graph evidence retained for later validation failures with error text/unwrapping intact.
- `internal/frontend/glfwgl/reload_test.go`: successful/latest-failure union replacement, success cleanup, bounded notices independent of retry state, active bundle preservation, and missing-include creation recovery without another primary edit.
- Existing dependency graph tests continue to cover content hashes, aliases, symlink retargets, deletion, debounce, concurrent edits, and generation-safe acknowledgement.

## Verification commands

```text
go test ./... -count=1
go test -tags headless ./... -count=1
go test -tags glfw ./... -count=1
go test -race -tags glfw ./internal/config ./internal/script ./internal/frontend/glfwgl -count=1
go vet ./internal/config ./internal/script
go vet -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm
go run ./scripts/check-maturity-gates.go
python -m json.tool docs/parity-support-matrix.json
```

## Deferred

Doctor/explain rendering and native multi-window scope routing remain later Phase 2 slices.
