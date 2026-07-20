# Phase 12 Accessibility Preflight

## Verdict

Proceed with constraints. Build accessibility from bounded immutable semantic documents and one projection-owned Windows native message router. Do not query mutable terminal/App state from UI Automation callbacks and do not install a second WndProc subclass.

## Locked architecture

- `internal/accessibility` owns platform-neutral detached documents, nodes, ranges, caret/selection, focus, revisions and coalesced semantic events.
- Logical grapheme order is authoritative for text/range offsets; rendered BiDi order is used only for screen rectangles.
- The initial privacy scope is the active visible viewport and focused modal/search input. Hidden tabs/windows/workspaces, scrollback, hyperlink targets, environment/process metadata and notification bodies are excluded.
- Per projection limits are 512 rows, 16,384 graphemes, 1 MiB UTF-8 and 256 nodes.
- UIA providers read only an atomically published immutable snapshot. Native ranges retain provider identity, generation and offsets—not terminal pointers.
- Focus precedence is modal, search, then focused terminal pane.
- Semantic events coalesce once per projection cycle and repaint-only damage produces no event. Overflow becomes one document-invalidated event.
- Initial providers are read-only. Native actions require a later decision and owner-thread stable-identity revalidation.
- IME and accessibility share one bounded deterministic native message router under the existing transactional WndProc host.
- Teardown stops publication, marks ranges stale, disconnects providers and unregisters accessibility before existing host restore/release, projection unbind and HWND destruction.
- Public activation is restart-scoped, Windows-only and default-off until Narrator/NVDA qualification passes.

## Feasibility evidence

CervTerm already has detached render/core snapshots, logical row text, BiDi permutation/inverse mapping, selection extraction, stable mux window/tab/pane/workspace values, modal activation IDs, owner-thread projection transactions and a fakeable WndProc host. These are sufficient inputs and lifecycle seams without native dependencies entering core, VT, render or mux.

Windows UI Automation requires an HWND root provider returned for `WM_GETOBJECT`, stable server-side provider/range objects, Text/Text2 document/visible/selection/caret semantics, bounding rectangles and explicit provider disconnection. UIA clients may invoke providers off the UI thread, which makes immutable publication—not direct App access—mandatory.

## Stop conditions

Stop a slice if it introduces mutable-state reads from native callbacks, unlimited history, sensitive hidden metadata, a second subclass, unbounded events, ambiguous callback/provider ownership, repaint-driven event floods, native types below the frontend, or a support/default-on claim without real assistive-technology evidence.

## Planned slices

1. Pure bounded semantic document.
2. Terminal logical text and visual bounds projection.
3. Window/tab/pane/modal/search focus tree.
4. Semantic revision/event coalescer.
5. Shared native message router preserving IME behavior.
6. Dormant UIA root/provider ABI.
7. Dormant TextPattern/TextPattern2 ranges.
8. Dormant projection lifecycle integration.
9. Strict default-off public activation.
10. Narrator/NVDA qualification and publication decision.

## Authoritative sources

- [Implementing UI Automation Text and TextRange providers](https://learn.microsoft.com/en-us/windows/win32/winauto/uiauto-implementingtextandtextrange)
- [About Text and TextRange control patterns](https://learn.microsoft.com/en-us/windows/win32/winauto/uiauto-about-text-and-textrange-patterns)
- [ITextProvider2 and caret ranges](https://learn.microsoft.com/en-us/windows/win32/api/uiautomationcore/nn-uiautomationcore-itextprovider2)
- [Handling WM_GETOBJECT](https://learn.microsoft.com/en-us/windows/win32/winauto/handling-the-wm-getobject-message)
- [UiaReturnRawElementProvider](https://learn.microsoft.com/en-us/windows/win32/api/uiautomationcoreapi/nf-uiautomationcoreapi-uiareturnrawelementprovider)
- [UI Automation threading issues](https://learn.microsoft.com/en-us/windows/win32/winauto/uiauto-threading)
- Microsoft Terminal `UiaTextRangeBase` implementation precedent.

## Rollback

Disable accessibility and restart. Remove native activation and router registration before provider ABI. Pure semantic packages may remain only when inert. Never regress existing IME routing while rolling accessibility back.
