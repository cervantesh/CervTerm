# Phase 11.8 — Default-off native IME activation

## Scope

This slice exposes strict restart-scoped `ime.enabled` configuration and transactionally activates the Phase 11 Windows decoder, candidate geometry and WndProc host for each initial, child and restored projection. The default remains `false`.

## Guarantees

- `ime.enabled` is a v2-only strict boolean, defaults to `false`, participates in includes/profiles/provenance/diffs, is emitted by the Lua template and Teal declarations, and never applies through live reload.
- Disabled Windows and every non-Windows projection perform no native IME call. Unsupported or failed activation leaves the existing GLFW character callback authoritative.
- Initial projections activate before controller adoption. The shared child projection preparation seam covers runtime-created and restored windows before publication.
- Each enabled Windows projection owns one decoder, candidate publisher binding and WndProc host through its existing pre-unbind transaction.
- WndProc installation failure does not fail window creation. Ambiguous native ownership remains attached for safe restore/release; clean failures fall back immediately.
- Successful teardown clears any visible candidate rectangle before restoring/releasing the native host, then continues through unbind, reverse resources and HWND destruction.
- Doctor reports configured intent, platform capability and that runtime activation is unavailable in headless diagnostic mode. Runtime failures use bounded user-facing messages without native payload disclosure.

## Deferred

- Real Japanese, Chinese and Korean IME qualification and documented evidence (Slice 11.9).
- Any conditional default-on decision (Slice 11.10). The setting remains opt-in until qualification passes.

## Required evidence

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -tags glfw ./internal/frontend/glfwgl -run 'TestIME|TestComposition|TestWindowsComposition|TestProjectionIME' -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go run ./scripts/check-maturity-gates.go
git diff --check
```
