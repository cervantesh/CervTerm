# Phase 12.7 — Dormant Windows TextPattern/TextPattern2

## Scope

Adds dormant, read-only `ITextProvider2` and `ITextRangeProvider2` COM surfaces to the Phase 12.6 UIA provider. Ranges are derived only from injected immutable visible accessibility documents. No App/config registration, event publication, selection mutation, scrolling, context menu, or production activation is introduced.

## Contracts

- Text-capable nodes advertise TextPattern (`10014`) and TextPattern2 (`10024`) through one derived provider interface with canonical element `IUnknown` identity.
- Document, visible, selection and caret ranges capture provider ID, generation, document instance, node and grapheme endpoints. Any replacement or newer publication makes historical ranges return `UIA_E_ELEMENTNOTAVAILABLE`.
- `ITextProvider`, `ITextProvider2`, `ITextRangeProvider` and `ITextRangeProvider2` use exact 9/11/21/22-slot SDK vtable order. By-value `UiaPoint` and `VARIANT` callbacks use C ABI thunks so 32-bit coordinates and stack layout are not truncated.
- Character means one UAX-29 grapheme. Format/word promote to line; paragraph/page promote to document. Endpoint crossing collapses the opposite endpoint. Range movement mutates only the private COM range.
- Text uses BSTR ownership and UTF-16-unit limits without splitting surrogate pairs. Visible bounds use owned `SAFEARRAY(VT_R8)` screen rectangles. Selection/visible/children arrays use `SAFEARRAY(VT_UNKNOWN)` and balanced COM ownership.
- Live ranges are bounded at 512, independently refcounted, and pin their enclosing native element/provider until final release. Clone creates independent mutable endpoints.
- Application mutations (`Select`, add/remove selection, scroll, context menu) return `UIA_E_NOTSUPPORTED`; attributes use UIA's reserved NotSupported value.
- The provider remains dormant and retains Phase 12.6 disconnect/final-release ordering.

## Evidence

Tests cover pure grapheme/line/document expansion and movement, endpoint crossing/copying, find/equality and staleness; exact text IIDs/vtable sizes/offsets; provider pattern identity; document/visible/selection/caret/point ranges; clone/compare/movement; BSTR text and UTF-16 surrogate limits; rectangle and unknown SAFEARRAY ownership; enclosing element ownership; reserved attributes; by-value ABI thunks; unsupported mutations; generation staleness; and final range release.

## Gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race -tags glfw ./internal/frontend/glfwgl -run 'TestNativeUIAText' -count=1
cd internal/frontend/glfwgl && GOOS=windows GOARCH=386 CGO_ENABLED=0 go test windows_uia_provider.go windows_uia_dispatcher.go windows_uia_native_windows.go windows_uia_native_objects_windows.go windows_uia_native_fragment_windows.go windows_uia_native_text_abi_windows.go windows_uia_native_text_objects_windows.go windows_uia_native_text_provider_windows.go windows_uia_native_text_range_windows.go windows_uia_point_thunk_windows_nocgo.go windows_uia_text_thunk_windows_nocgo.go windows_wndproc_host.go
go run ./scripts/check-maturity-gates.go
git diff --check
```
