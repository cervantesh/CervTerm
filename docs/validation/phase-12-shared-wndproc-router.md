# Phase 12.5 — Shared Windows WndProc router

## Scope

Generalizes the dormant Windows WndProc host from one IMM decoder to a bounded deterministic handler router. Existing IME construction, activation and cleanup continue through the same host. No accessibility provider or production accessibility registration is added.

## Contracts

- At most eight handlers, including the legacy IMM decoder, are retained in registration order with nonzero non-reused IDs.
- Duplicate handler identity, stale removal, capacity and ID exhaustion fail explicitly without mutating installed ownership.
- The first handled result consumes exactly once. If none handles, the captured prior WndProc is chained exactly once.
- Handler errors and panics are reported and contained independently. Legacy IMM panic behavior is preserved: composition messages consume; other messages chain.
- Dispatch traverses a detached registration snapshot. Reentrant unregister marks entries inactive and compacts host ownership immediately, allowing bounded replacement; handlers registered during dispatch begin on the next message. Reentrant restore/release overrides a handled result, stops later handlers and chains through the callback's captured prior procedure.
- Installation conflict, rollback, later subclass ownership loss, retryable restore, callback release and teardown ordering retain the Phase 11 transactional behavior.
- Deactivation prevents handler delivery before restoration; release clears decoder and handler references only after callback ownership is no longer installed.

## Evidence

The unchanged Phase 11 WndProc/IME suite plus new tests cover deterministic order, first-consume, one prior chain, duplicate/stale removal, eight-handler bound, ID exhaustion, reentrant pending-handler removal, individual error/panic containment and handler-only hosts.

## Gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go run ./scripts/check-maturity-gates.go
git diff --check
```
