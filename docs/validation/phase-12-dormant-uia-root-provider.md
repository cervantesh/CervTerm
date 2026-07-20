# Phase 12.6 — Dormant Windows UIA root provider

## Scope

Adds a fakeable, dormant Windows UI Automation root/fragment provider over injected immutable `accessibility.Document` publication. It extends the shared WndProc router to carry full HWND/message/WPARAM/LPARAM values and handler results. No `App` reference, config option, production registration or event publication is introduced.

## Contracts

- Publication is atomic and immutable. Disconnect removes the document before native ownership is released; provider calls then return `UIA_E_ELEMENTNOTAVAILABLE`.
- The process-wide dispatcher retains at most 64 providers with nonzero stable tokens, duplicate/stale/capacity checks and one registration reference.
- `QueryInterface` supports IUnknown, IRawElementProviderSimple, Fragment and FragmentRoot with shared COM identity. Atomic references saturate and never underflow.
- Native vtables use exact interface order. GUIDs, HRESULTs, pointer-sized interface fields, VARIANT/BSTR and SAFEARRAY runtime-ID ownership are explicit.
- Properties, bounds, fragment navigation, point lookup and focus derive only from detached accessibility nodes. Unknown properties return an empty VARIANT; unsupported mutation returns `UIA_E_NOTSUPPORTED`.
- `WM_GETOBJECT` consumes only `UiaRootObjectId` for the matching HWND and registered, connected provider. It passes the raw COM interface pointer to `UiaReturnRawElementProvider`; all other messages chain exactly once.
- The Windows bridge is dormant: constructors and handlers are not referenced by projection/App startup. Existing IMM registration and behavior remain unchanged through a compatibility adapter.

## Evidence

Tests cover distinct Fragment/FragmentRoot COM ABI and identity, refcount matrices/saturation/underflow, provider options/host provider/properties including rectangle SAFEARRAY ownership, bounds/navigation/focus/disconnect, lazy ref-aware historical-node pruning, 64-provider bound and ownership, WM_GETOBJECT consume/chain/panic/concurrent-disconnect lifetime pinning, concurrent reads, exact GUID/HRESULT/vtable/VARIANT layout, native QueryInterface/navigation/focus/runtime SAFEARRAY/point lookup and raw COM pointer forwarding. A CGO-free selected-file Windows 386 compile verifies the provider, native ABI and WndProc router independently of GLFW/OpenGL linkage.

## Gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
GOOS=windows GOARCH=386 CGO_ENABLED=0 go test windows_uia_provider.go windows_uia_native_windows.go windows_uia_native_objects_windows.go windows_uia_native_fragment_windows.go windows_uia_point_thunk_windows_nocgo.go windows_wndproc_host.go
go run ./scripts/check-maturity-gates.go
git diff --check
```
