# Phase 2 Dependency Graph Watch Validation

## Scope

Replace primary-only polling with dependency-aware watching for the active evaluated configuration graph. Preserve atomic bundle activation and last-known-good failure behavior. Runtime-callback-only module discovery, desired/effective diff classification, and CLI wiring remain deferred.

## Verified contracts

- Active watch paths contain the selected primary, every declarative include, every selected symlink alias, and local modules observed through `require`, `dofile`, or `loadfile` during evaluation.
- Staged/generated Teal Lua is excluded, preventing publication-triggered reload loops.
- Source watch digests bind canonical target identity to the exact bytes loaded; symlink retargets are detected even when both targets contain identical bytes.
- Lua declarative sources execute from the already-hashed bytes rather than reopening the file.
- Dependency wrappers record before and after the standard loader call. If a module mutates while loading, the before hash remains authoritative and forces a newer generation.
- Canonically deduplicated declarative sources retain every selected alias. Dependency records distinguish aliases that resolve to the same target.
- The watcher hashes content, observes missing files, normalizes duplicates, and debounces/coalesces the entire graph only after it is stable.
- Reload snapshots the old active graph before and after evaluation, verifies all new candidate hashes before and after commit, and queues another reload when any generation changes in those windows.
- A successful activation atomically replaces the watched graph. Failed validation/preparation/publication leaves the prior active graph and owner intact.
- V1 and v2 startup both verify captured hashes before entering the event loop.

## Focused evidence

`internal/frontend/glfwgl/reload_test.go` covers:

- graph-wide debounce and dependency deletion;
- coalescing edits across multiple files;
- active include changes during primary evaluation;
- newly introduced include mutation during its own evaluation;
- module mutation during `require`;
- primary, duplicate-include, and dependency symlink retargets with identical content.

`internal/config/source_graph_test.go` and `source_dependencies_test.go` retain graph limits, cycle, capture, collision, and strict-v2 coverage.

## Commands

```text
go test ./... -count=1
go test -tags headless ./... -count=1
go test -tags glfw ./... -count=1
go test -race -tags glfw ./internal/config ./internal/script ./internal/frontend/glfwgl -count=1
go vet ./internal/config ./internal/script
go vet -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm
python -m json.tool docs/parity-support-matrix.json
```

Adversarial review found and drove fixes for newly introduced dependency races, dependency-loader TOCTOU, v1 startup verification, and discarded declarative/dependency symlink aliases. Final targeted review reported **NO BLOCKERS**.

## Deferred boundary

Modules first loaded later from runtime callbacks are not evaluation dependencies and are not automatically watched. A later runtime-scope slice must define whether such dynamic discovery mutates the active watch graph or remains manual-reload-only.
