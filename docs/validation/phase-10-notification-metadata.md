# Phase 10.6a Validation — Notification Metadata

## Contract

- OSC 9 and OSC 777 `notify` parse into metadata only; no native API is reachable from VT/core/mux.
- Titles are at most 256 bytes, bodies at most 4096 bytes, UTF-8 is required, and C0/DEL payloads reject atomically.
- Each pane retains at most 32 requests in a lazily allocated ring with monotonic sequence identity.
- Mux emits addressed detached request events and an explicit overflow event when one parser batch exceeds retention.
- Request title/body never enters diagnostics or command execution.

## Automated evidence

- `go test ./internal/core ./internal/vt ./internal/mux -count=1`
- `go test ./... -count=1`
- `go test -tags glfw ./... -count=1`
- `go vet -unsafeptr=false ./...`
- `go vet -unsafeptr=false -tags glfw ./...`
- `go test -race ./... -count=1`
- `go run ./scripts/check-maturity-gates.go`

Native notification policy, consent/rate limiting, focus rules, and platform adapters intentionally follow in Phase 10.6b; this slice cannot produce an external effect.
