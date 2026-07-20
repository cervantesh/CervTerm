# Phase 11.6 — Dormant Windows IMM decoder

## Scope

This slice adds a build-tagged, fakeable IMM message decoder and Windows `imm32.dll` adapter. It remains dormant: no WndProc is subclassed, no application startup path constructs the decoder, and GLFW character delivery remains authoritative.

## Guarantees

- `WM_IME_STARTCOMPOSITION`, `WM_IME_COMPOSITION`, and `WM_IME_ENDCOMPOSITION` are classified without consuming unrelated messages.
- Every composition or candidate operation pairs one successful `ImmGetContext` with one attempted `ImmReleaseContext`.
- Result/preedit strings and attributes use exact, bounded two-pass reads. Negative, odd, oversized, short, drifting cursor, mismatched attribute, and malformed UTF-16 payloads fail without partial preedit/result routing.
- A combined result and new preedit is fully read first, then ordered as old-generation commit, echo arm, new-generation start, and update.
- Native result echo suppression is armed only after successful whole-text routing and clears on completion, mismatch, timeout, non-echo input, or focus loss.
- Candidate calls use checked 32-bit native coordinates. Native/callback panics are contained and returned as errors.
- ABI constants, signed `LONG` conversion, `CANDIDATEFORM` layout and return conventions are tested behind `glfw && windows`.

## Explicitly deferred

- WndProc installation, restoration, chaining, HWND ownership and callback lifetime (Slice 11.7).
- Configuration, startup activation and fallback policy (Slice 11.8).
- Default enablement and real Japanese/Chinese/Korean IME qualification (Slices 11.9–11.10).

## Required evidence

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -tags glfw ./internal/frontend/glfwgl -run 'TestIMM|TestComposition|TestWindowsIMM' -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go run ./scripts/check-maturity-gates.go
git diff --check
```
