# Phase 2 Desired/Effective Config Scope Validation

## Scope

Replace the generic restart boolean with exhaustive per-leaf application scopes and detached desired/effective frontend state. Apply cursor policy live and shell settings to future panes. Durable runtime patch scopes/provenance, CLI wiring, doctor/explain integration, and multi-window realization remain deferred.

## Application contract

Every public schema leaf has exactly one scope:

- `live`: window opacity/blur, all colors, scrolling, scrollbar, cursor policy, keys, and events;
- `new_pane`: all shell fields;
- `new_window`: initial width and height;
- `window_recreate`: reserved as an explicit capability, currently unused;
- `restart`: padding, dynamic-title policy, font/resource settings, clipboard policy, and render/cached-hotkey policy.

Tables and `config_version` are not leaf application targets. Composition metadata remains candidate-only metadata.

## Verified behavior

- `DiffConfig` compares all 51 `Config` leaves in deterministic schema order and stores only path/scope, never values.
- The schema completeness test derives expected changes from `SchemaFields` and fails if an available leaf lacks a scope or the diff omits it.
- `Config.Clone`, frontend startup ingestion, desired/effective accessors, and live merge detach mutable shell argument/environment state.
- Successful reload stores the complete candidate as desired, applies only live fields to effective, and retains exact non-live pending paths.
- Cursor shape/blink/interval/thickness now apply live through existing draw/loop consumers.
- Shell changes do not alter existing pane processes; newly split panes receive detached desired program/arguments/directory/environment values.
- Runtime live setters update desired and effective live values without erasing unrelated pending shell/font changes.
- Failed reload preserves desired, effective, pending, runtime, and bundle state while retaining the last error.
- A successful recovery clears the last error; reverting desired fields clears pending records and returns the plain reload notice.
- Scoped notices are bounded and include only path plus scope; sensitive `shell.env` values cannot appear.

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

Independent review found and drove correction of startup mutable ownership. Runtime override capability metadata was deliberately removed because this slice does not implement ADR-0002's durable `ConfigScopeID` patch model. Final blocker review reported **NO BLOCKERS**.

## Deferred boundary

Current Lua setters retain their established synchronous live behavior, but file reload remains authoritative over those values. A later bounded slice must implement typed `ConfigScopeID` patches, provenance, revalidation, and survival across successful reload before claiming the durable runtime-scope portion of ADR-0002.
