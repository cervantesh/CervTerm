# Phase 2 Runtime Config Scope Validation

## Scope

Implement ADR-0002 process-local `ConfigScopeID` patches for the existing live Lua configuration setters. Preserve synchronous callback visibility, survive/revalidate across reload, and retain value-free runtime provenance. Executable CLI flags, doctor/explain rendering, non-live runtime setter surfaces, and multi-window scope routing remain deferred.

## Verified contracts

- The frontend allocates one opaque scope at startup and closes/removes it with `App`; stale or closed IDs reject mutation and application.
- Schema metadata permits runtime patches only for the current setters: opacity, blur, background, scrolling, and scrollbar leaves. Sensitive, scripting, composition, selection, cursor, and other unsupported paths reject.
- Existing typed setter Config values adapt to ordered `RuntimeOverride{Path, Value}` entries. Runtime raw values use `resolveCLIOverridePath` and `decodeCLIOverrideValue`, the same coercion/unknown-path decoder as CLI overrides.
- Each setter call proposes one detached patch transaction, validates the fully composed+scoped desired config, prepares fallible frontend resources, commits effective state, then commits scope ownership before returning.
- Failure before commit preserves the prior patch, desired/effective state, and active resources. Independent successful calls remain committed if later calls fail; the last successful transaction for a field wins.
- Runtime patches do not mutate the composed base. Successful file reload reapplies and validates them before Teal publication or bundle transfer.
- An incompatible composed candidate plus retained patch rejects the entire reload and preserves config/runtime/bundle/records.
- Path-specific or all-path clearing restores the latest composed value through the same prepare/commit transaction.
- Runtime records contain only dotted path plus scope ID. Their provenance overlay uses `LayerRuntime`, carries `ConfigScopeID`, and appends the prior composed winner to its existing low-to-high overwrite chain.
- Existing pane-local font-size/zoom remains action state outside composed runtime patches.

## Test evidence

`internal/config/runtime_scope_test.go` covers:

- scope lifecycle and stale rejection;
- ordered raw typed decoding and unsupported/sensitive paths;
- last-successful transaction behavior;
- reload reapplication and invalid-combination rollback;
- partial/all clearing;
- exact schema capability surface;
- runtime provenance overwrite chains.

`internal/frontend/glfwgl/reload_test.go` covers synchronous live setter state, preservation of unrelated pending values, file-reload survival, invalid candidate preservation, explicit clearing, and app-level runtime provenance.

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

Adversarial ADR comparison found and drove fixes for shared-decoder parity and composed provenance-chain retention. Final targeted review reported **NO BLOCKERS**.

## Deferred boundary

The current process has one frontend/window scope, so focused and origin-bound callbacks resolve to the same owner. ADR-0004 multi-window work must map focused/origin window identity to this unchanged patch API. CLI flags, persistent explain output, and doctor rendering follow in later Phase 2 slices.
