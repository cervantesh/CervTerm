# Phase 11.7 — Dormant transactional Windows WndProc host

## Scope

This slice adds a fakeable WndProc subclass host and Win32 adapter. It remains dormant: no startup or projection factory constructs it, no configuration activates it, and GLFW character callbacks remain authoritative.

## Guarantees

- Installation runs synchronously on the existing GLFW owner-thread contract and retains the Go callback, exact HWND and exact prior WndProc strongly for the complete installed lifetime. The native adapter allocates one process-wide callback thunk and uses a bounded active-HWND registry, avoiding permanent per-window callback-table growth.
- `GetWindowLongPtrW` and `SetWindowLongPtrW` clear last error before each call. A zero return is an error only when the newly captured last error is nonzero; nonzero returns ignore stale last-error values.
- Installation is transactional. A changed prior procedure triggers rollback; rollback must prove which procedure it displaced. Any failed or ambiguous rollback retains callback ownership and prevents unsafe release.
- Active, matching-HWND decoder-handled messages are consumed. Unhandled, inactive, mismatched-HWND and restored delivery chains through the exact saved prior procedure. Decoder/callback/chain/report panics cannot cross the Win32 ABI.
- Restore deactivates first, refuses to overwrite a later subclass, restores only when the installed callback is still current, and remains retry-safe/idempotent. Release refuses to discard a callback that might still be installed.
- The existing pre-unbind order remains cancel → deactivate → restore → release → unbind → reverse resources → HWND destruction. Every cleanup step is attempted even if an earlier callback panics.

## Explicitly deferred

- Production construction, WndProc installation and candidate callback binding.
- `ime.enabled` configuration, fallback policy and diagnostics (Slice 11.8).
- Real Japanese/Chinese/Korean IME qualification and any default-on decision (Slices 11.9–11.10).

## Required evidence

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -tags glfw ./internal/frontend/glfwgl -run 'TestIME|TestComposition|TestWindowsComposition|TestDormantWndProc|TestNativeWndProc' -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go run ./scripts/check-maturity-gates.go
git diff --check
```
