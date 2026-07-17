# Phase 2 Candidate Bundle Validation

## Scope

Candidate-only ownership and lifecycle for composed v2 configuration. Public `Load`, frontend installation, graph watching, and desired/effective behavior remain unchanged.

## Verified contracts

- One bundle owns the validated resolved Config, one candidate Lua state/runtime, effective bindings/events, primary imperative registrations, selection/provenance, dependency graph/staging, and deferred Teal publication.
- Every source's legacy fail-fast key/event surface validates before effective merge, even when a higher layer would replace it.
- Build/config/compose/script validation failures close the Lua state and remove candidate-owned staging.
- Caller base maps/lists and returned Config/selection/provenance/dependency/publication values cannot mutate stored bundle state.
- Teal publication is deferred, retryable after failure, and idempotent after success.
- `Close` is idempotent and releases runtime before graph staging.
- Lifecycle methods are serialized/main-thread-only; runtime transfer is deliberately withheld until the frontend atomic-install slice.
- Existing public loading remains fail-closed for composition metadata.

## Evidence

```text
go test ./internal/script -run CandidateBundle -count=1 PASS
```

Independent review found and verified fixes for caller-base aliasing, mutable publication results, and premature runtime exposure. Final blocker review reported **NO BLOCKERS**.

## Activation gate

The next slice must prepare every fallible frontend live resource first, then publish Teal and transfer the complete bundle through one mechanically infallible main-thread commit. Until then no active path consumes this bundle.
