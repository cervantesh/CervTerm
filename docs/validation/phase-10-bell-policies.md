# Phase 10.5 Validation — Bell Policies

## Contract

- BEL observation remains lossless in core, mux, and Lua regardless of sink policy.
- Strict v2 live policy selects `disabled`, `audible`, `visual`, or `taskbar`, with bounded focus/throttle/duration controls.
- Only frontend sinks are throttled; effects execute on the locked OS-thread event path.
- Visual expiry wakes on-demand rendering once; taskbar attention is GLFW-owned; Windows audible mode uses `MessageBeep`.
- Default mode is disabled and unsupported native audio fails closed.

## Automated evidence

- `go test ./internal/bellpolicy ./internal/config -count=1`
- `go test -tags glfw ./internal/frontend/glfwgl -run 'TestBell|TestVisualBell' -count=1`
- `go test ./... -count=1`
- `go test -tags glfw ./... -count=1`
- `go vet -unsafeptr=false ./...`
- `go vet -unsafeptr=false -tags glfw ./...`
- `go test -race ./... -count=1`
- `go run ./scripts/check-maturity-gates.go`

## Manual qualification

On Windows, configure each mode, emit `printf '\a'`, verify focus filtering and burst throttling, and confirm that an `events.bell` counter increments once per BEL even when effects are disabled or suppressed.
