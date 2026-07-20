# Phase 11 Preflight — IME and Preedit

## Disposition

Proceed with constraints from local `dev` merge `934894e`. ADR-0005 is accepted as a family of narrow projection-owned capabilities. Phase 11 adds composition only; accessibility remains Phase 12. No renderer/backend selection or mux ownership change is involved.

Production behavior remains unchanged in Slice 11.0. Native activation will first publish restart-scoped `ime.enabled=false`; default-on is conditional on real Windows Japanese/Chinese/Korean qualification.

## Existing path and gap

Current text input is `GLFW SetCharCallback -> retained modal/search or input.Encode(rune) -> focused mux pane -> PTY`. GLFW exposes no composition start/update/result/cancel event, preedit selection/caret, or candidate rectangle. `suppressNextChar` is a one-rune binding workaround and cannot prove exactly-once multi-rune commits.

Each native window already has a projection-local `App`, and one locked OS thread owns GLFW/native calls. Projection bundles close resources before destroying HWNDs, providing the ownership base for a composition host.

## Windows feasibility contract

A `glfw && windows` host may obtain the HWND and transactionally subclass `GWLP_WNDPROC`:

- keep the exact prior WndProc and chain every unhandled message with `CallWindowProcW`;
- interpret a zero `SetWindowLongPtrW` return using last-error semantics;
- strongly own the callback/HWND association, contain callback panics, and restore before unbind/HWND destruction;
- pair every `ImmGetContext` with `ImmReleaseContext`;
- use bounded two-pass `ImmGetCompositionStringW` reads for result/preedit/attributes/cursor;
- derive the target span from target composition attributes and collapse to the cursor when absent;
- reject odd lengths, malformed UTF-16, offset drift, short reads and bound overflow atomically;
- publish candidate/composition positions using checked framebuffer-size/window-size ratios per axis, never content scale as a coordinate substitute.

## Locked lifecycle and bounds

`Inactive -> Started(stable target activation) -> Updated(preedit,cursor,target span)* -> Committed(text) -> Inactive`.

Malformed data, focus/target/window/workspace/modal/search drift, disable or teardown follows cancel. Modal and search require activation IDs distinct from content revision and whole-string handlers. Echo suppression executes before modal/search/terminal routing, matches the complete bounded result sequence, and expires after 100 ms (fake-clock tested) or immediately on mismatch, non-echo key input, focus loss or teardown.

Bounds: 16 KiB UTF-8/4096 runes for preedit; 64 KiB UTF-8 for commit. Preedit is absent from PTY, core cells, mux/render snapshots, scripting and persistence.

One `nativeProjectionBundle.beforeUnbind` coordinator executes:

`cancel -> deactivate callback delivery -> restore prior WndProc -> release host/context -> unbind -> remaining resources -> Destroy HWND`.

## Approved slices

1. 11.1 pure bounded composition/UTF-16 state;
2. 11.2 stable modal/search activation IDs and whole-text router;
3. 11.3 fake-host coordinator and lifecycle cancellation;
4. 11.4 preedit rendering/damage/idle;
5. 11.5 pure candidate geometry/invalidation;
6. 11.6 dormant fakeable Windows IMM decoder/echo model;
7. 11.7 dormant transactional WndProc host/pre-unbind lifecycle;
8. 11.8 strict default-off config/Teal/template/doctor and opt-in activation;
9. 11.9 real Windows qualification/docs/validation/drift;
10. 11.10 conditional default-on only after required native passes.

## Challenge record

Independent same-engine design review initially found mutable modal/search revisions, echo suppression ordering, post-unbind cleanup, incomplete IMM32 lifecycle rules, and incorrect content-scale geometry assumptions. All were repaired and final review found no design blocker.

Independent plan review required default-off-before-qualification, smaller atomic slices, stable activation IDs, one teardown order, exact commands, restart fallback, and a named CI gate. The revised plan identifies `.github/workflows/ci.yml`, Windows job/check `test`, step `GLFW tests`, and the exact command. Remaining constraints are recorded; no blocker remains.

## Per-slice gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race ./... -count=1
go run ./scripts/check-maturity-gates.go
git diff --check
```

Windows-native slices additionally run:

```text
go test -tags glfw ./internal/frontend/glfwgl -run 'TestIME|TestComposition|TestWindowsComposition' -count=1
```

Cross-compilation is compile evidence only. Real Windows IME checks and GitHub job `test` are required before remote/release support or default-on publication.

## Required default-on matrix

Slice 11.9 records evidence in `docs/validation/phase-11-ime-preedit.md` plus bounded screenshots/logs. Default-on requires Windows 11 with the built-in Microsoft Japanese IME, Microsoft Pinyin (Simplified Chinese), and Microsoft Korean IME installed; an unavailable required IME is a skip that blocks Slice 11.10.

For each IME, compose and commit `日本語`, `中文`, or `한글` in a terminal and prove shell-captured UTF-8 contains exactly one copy. At least one IME must also pass search and retained-modal whole-text commit. Required common cases are: visible preedit/target span/caret; candidate UI anchored to the active caret at base zoom and pane-local zoom; focus-loss/modal-open/pane-switch cancellation with zero PTY bytes; window close/restoration without stale callbacks; and legacy plain GLFW characters when disabled. Supplementary/combining text and 100%/125%/150% scale are required where the installed IME/monitor supports them; unavailable scale/second-monitor cases are explicit skips but do not replace the three-IME terminal commit requirement.

Every required row records OS build, IME name/version, config, action, expected/actual result and evidence path. Any failed required row blocks default-on and remote/release support; Phase 11 remains documented opt-in.

## Rollback

Set `ime.enabled=false` and restart. Install failure falls back immediately without partial host publication. Reverse activation/default, host, decoder, geometry, rendering, coordinator, routing/activation IDs, then pure state. No persisted migration is involved.
