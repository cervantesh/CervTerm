# Phase 11.3 Validation — Composition Coordinator and Lifecycle Cancellation

## Contract

- A projection-local fake-event coordinator owns the pure `ime.Controller`, captures one stable modal/search/pane target at start, applies normalized preedit updates without PTY I/O, and routes one detached whole-text commit through the Phase 11.2 router.
- Malformed or over-bound native payloads cancel atomically with `CancelMalformed`. Stale generations, inactive delivery and route failures are returned; a completed commit is never retried implicitly, preventing duplicate delivery.
- Cancellation is idempotent and main-thread-local. Pane/focus changes, matching pane close/transfer, active target reconciliation, modal open/replace/close, search open/close, native/projection focus loss, source tab/pane transfer, workspace hide and projection teardown cancel with the documented reason.
- Unrelated inactive-tab lifecycle and unrelated pane transfers preserve composition. Failed cross-window transfers do not cancel the source.
- Every production projection bundle owns one at-most-once `beforeUnbind` coordinator. Slice 11.3 populates `cancel -> deactivate callback delivery`; fake hooks prove the reserved full order `cancel -> deactivate -> restore host callback -> release host/context -> unbind -> reverse resources -> Destroy`. The restore/release slots remain deliberate no-ops until the dormant native host slices. Restore rollback uses the same helper instead of bypassing it.
- Cleanup is best effort and at-most-once: errors are joined while later teardown stages continue. Native callback/context cleanup is not retried after a partial attempt because retrying restored callbacks or released handles is unsafe.
- This slice adds no native IME adapter, rendering, candidate geometry, config surface or external side effect. Legacy GLFW characters remain authoritative.

## Evidence

Automated tests cover preedit isolation, one-call UTF-8 commit and duplicate rejection, malformed cancellation, unchanged/changed target reconciliation, delivery deactivation, route failure without retry, modal/search/focus/mux cancellation, unrelated-event preservation, successful/failed source transfer behavior, workspace hiding, projection focus/close, restore-style early unbind, and exact idempotent teardown ordering.

## Required gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race ./... -count=1
go run ./scripts/check-maturity-gates.go
git diff --check
```

## Rollback

Remove the frontend composition coordinator and lifecycle wrappers, restore direct modal/search calls and the previous projection unbind order, then remove the `beforeUnbind` seam. `internal/ime` and the Phase 11.2 text router remain valid foundations; no persisted state or native hook requires migration.
