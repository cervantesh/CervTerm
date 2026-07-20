# Phase 10.6b Validation — Notification Policy Boundary

## Contract

- Native notification consent is strict v2, live, and disabled by default.
- Fresh requests must pass explicit enablement, `always|unfocused` focus policy, and a bounded per-window minimum interval.
- Requests queued before a native projection exists irrevocably lose freshness and cannot cause delayed effects.
- The frontend adapter seam executes only from addressed OS-thread mux dispatch and is fakeable in tests.
- Unsupported-adapter and overflow diagnostics are once-only and never include title/body or adapter error payloads.

## Automated evidence

- `go test ./internal/notificationpolicy ./internal/config ./internal/mux -count=1`
- `go test -tags glfw ./internal/frontend/glfwgl -run 'TestNotification|TestPendingNotification' -count=1`
- `go test ./... -count=1`
- `go test -tags glfw ./... -count=1`
- `go vet -unsafeptr=false ./...`
- `go vet -unsafeptr=false -tags glfw ./...`
- `go test -race ./... -count=1`
- `go run ./scripts/check-maturity-gates.go`

The default platform sink remains fail-closed in this slice. A separately qualified Windows native adapter must replace it before Phase 10.6 is marked supported.
