# Phase 10.6c Validation — Windows Native Notifications

## Contract

- Only requests accepted by the default-off Phase 10.6b policy reach the adapter.
- Windows uses `Shell_NotifyIconW` with a projection HWND, bounded Unicode title/body fields, `NIF_REALTIME`, and `NIIF_RESPECT_QUIET_TIME`.
- Notification-area icon add/modify/delete is transactional and projection-owned; modify failure deletes the provisional icon, live consent withdrawal cleans it up, shutdown/rollback removes it, and failed deletion retains ownership for a bounded retry.
- Adapter errors remain generic and payload-redacted at the caller boundary.
- Non-Windows builds retain the fail-closed adapter.

## Automated evidence

- `go test -tags glfw ./internal/frontend/glfwgl -run 'TestNotification|TestPendingNotification|TestWindowsNotification' -count=1` (includes consent-withdrawal cleanup, delete-failure ownership retention, and retry)
- `go test ./... -count=1`
- `go test -tags glfw ./... -count=1`
- `go vet -unsafeptr=false ./...`
- `go vet -unsafeptr=false -tags glfw ./...`
- `go test -race ./... -count=1`
- `go run ./scripts/check-maturity-gates.go`

## Manual Windows matrix

With `config_version=2` and `notification.enabled=true`, emit OSC 9/777 while unfocused and confirm one native balloon. Confirm focused suppression, configured rate limiting, quiet-time behavior, Unicode truncation, no delayed balloon after creating another window, and tray-icon removal on close. Repeat with the default configuration and confirm no native effect.
