# Phase 2 Atomic Bundle Activation Validation

## Scope

Activate explicitly authored `config_version = 2` composition at executable startup and manual/primary-file reload. Preserve omitted/authored-v1 single-source behavior. Dependency-graph watching, CLI override wiring, runtime scopes, and desired/effective diff classification remain deferred.

## Verified contracts

- `LoadVersioned` evaluates the selected Lua/Teal source once and dispatches on `Document.AuthoredVersion`, never migrated version or source-text heuristics.
- Authored v1 retains last-return semantics, user replacements of `require`/`dofile`/`loadfile`/`package.loaders`, selected source identity, marker-free adjacent `tl gen`, and legacy Runtime ownership.
- Explicit v2 retains one candidate Lua state plus composed Config, bindings/events, imperative stores, graph/staging, selection/provenance, and dependencies.
- Startup constructs GLFW, GL, renderer, and atlas resources before Teal publication; only then does it commit the runtime, initialize action bindings, and spawn the PTY.
- Reload prepares all fallible raster contexts without mutating config, pane UI, mux policy, runtime ownership, or active atlas maps.
- After preparation, v2 publication precedes one non-error-returning main-thread config/runtime/bundle commit. The prior owner closes only after the new owner is installed.
- Publication faults and raster preparation faults preserve the exact active config/runtime/bundle and close candidate resources.
- A v2-owned Teal-to-v1 transition journals generated Lua and ownership marker bytes. Startup/reload failure restores both; successful activation disarms the journal and remains marker-free.
- Candidate activation handles are allocation-complete and one-shot; bundle closure invalidates uncommitted handles.

## Focused evidence

- `internal/script/versioned_load_test.go`
  - authored-v1 and explicit-v2 dispatch;
  - exact-once evaluation;
  - v1 last return and deferred global replacement;
  - v2-to-v1 marker transition.
- `internal/frontend/glfwgl/reload_test.go`
  - preparation does not mutate active state and abort closes resources;
  - commit transfers prepared resources;
  - explicit-v2 bundle activation;
  - v2 publication fault rollback;
  - v2-to-v1 raster-preparation fault restores generated Lua, marker, and active ownership.
- `internal/script/candidate_bundle_test.go`
  - activation and closure lifecycle.

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

Independent adversarial review initially identified startup ordering, v1 compatibility, marker transition, preparation mutation, and rollback gaps. Each was fixed and regression-tested. Final targeted review reported **NO BLOCKERS**.

## Remaining Phase 2 gates

- Watch the complete canonical dependency graph rather than only the primary source.
- Wire environment/profile/CLI selection inputs into executable flags and diagnostics.
- Add desired/effective/restart diff classification and runtime/window scopes.
