# Phase 12.8 — Dormant accessibility projection lifecycle

## Scope

Adds a nil-by-default, internal accessibility lifecycle factory to initial, runtime-child and restored projection preparation. Production remains dormant: no config, App startup assignment, native provider construction, WndProc registration or event publication occurs unless tests inject the factory.

## Contracts

- Initial adoption and the shared runtime/restore projection factory prepare accessibility before bundle publication; partial factory results close transactionally.
- A completed lifecycle is owned by `compositionBeforeUnbind` and closes after composition cancellation but before native handler deactivation/WndProc restore and release.
- Windows preparation publishes one immutable document, constructs logical/native UIA providers, registers a bounded dispatcher token, registers UIA on the projection's existing shared WndProc host, and installs a host only when IME has not already done so.
- Teardown is at-most-once: disconnect/stale native publication, unregister UIA handler, unregister dispatcher/drop ownership, then existing host deactivate/restore/release, mux unbind, reverse resources and HWND destruction.
- If exact prior-WndProc restoration cannot be proven, the pre-existing host safety contract reports the error and retains callback registry ownership until process teardown rather than releasing a callback that the live HWND may still target; later cleanup steps and HWND destruction still run.
- Failed capture/publication retains the last immutable document. Native-event failure occurs after successful publication and does not roll the document back.
- Runtime children and restored windows share `glfwProjectionFactory.prepareProjection`; every candidate therefore receives independent lifecycle ownership and existing bundle rollback semantics.
- Existing IME uses the same host and preserves behavior/order. No second subclass is installed.

## Evidence

Tests cover nil-by-default factory behavior, completed and partial transfer, exact pre-native teardown order, initial projection adoption, actual provider/dispatcher/handler preparation, shared installed IME host reuse, fallback/inactive host rejection without IME retry, duplicate publication retention, capture failure retention, native-event failure retention, host-install rollback, COM/token release, normal callback release, ambiguous callback-retention safety, and existing IME/composition suites.

## Gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race -tags glfw ./internal/frontend/glfwgl -run 'Test(ProjectionAccessibility|DormantProjectionAccessibility|InitialProjectionTransfers)' -count=1
go run ./scripts/check-maturity-gates.go
git diff --check
```
