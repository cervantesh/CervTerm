# Phase 8 Visible Tabs — Validation

## Delivered slices

| Slice | Evidence |
|---|---|
| Ordered tab model | PR #176, merge `2a99200` |
| Transactional tab lifecycle | PR #177, merge `812ec56` |
| Cross-tab pane transfer | PR #178, merge `1a57408` |
| Retained tab bar/configuration | PR #179, merge `cda888e` |
| Typed tab actions | PR #180, merge `9f6199f` |
| Chrome interaction/confirmation | PR #181, merge `c05e572` |

## Automated qualification

Each implementation slice passed its focused tests and the repository gates before merge. Final close-out reruns:

```text
go run ./scripts/check-maturity-gates.go
go test ./... -count=1
go test -tags glfw ./internal/frontend/glfwgl -count=1
go vet -unsafeptr=false ./...
go test -race ./internal/action ./internal/config ./internal/modal ./internal/mux ./internal/script ./internal/frontend/glfwgl -count=1
```

Coverage includes ordered/global identity invariants, 256-tab bounds, provisional spawn rollback, deterministic close despite session errors, nested transfer and failure rollback, per-pane metrics, inactive projection, top/bottom startup/runtime geometry, scrollbar coexistence, retained overflow/hits, Unicode cluster clipping, live configuration/runtime scopes, stable-ID action targeting, close-confirmation revision invalidation, lifecycle revision changes and activity badges.

## Compatibility and ownership

- The default `tab_bar.mode = "multiple"` reserves zero pixels for one tab.
- Pane PTY/session/parser/terminal identity is preserved across transfer; no transfer spawns or closes a process.
- Inactive tabs receive no terminal input, active render projection or PTY resize.
- GLFW/OpenGL/native window calls remain on the OS thread; mux/config/action packages do not import GLFW.
- Renderer selection remains excluded.

## Platform qualification

- Windows: automated Go/default/GLFW/race gates pass; the interactive matrix is documented in `docs/manual-verification.md#phase-8-visible-tabs-qualification` for packaged daily-driver verification.
- Linux: headless CI build/tests pass.
- macOS and Linux GUI: not manually qualified in this close-out; no additional GUI support claim is made.

## Rollback

Public tab behavior can be disabled with `tab_bar.mode = "hidden"`. Revert interaction/actions/bar slices before lifecycle/model foundations. One-tab hidden mode preserves the pre-Phase-8 compatibility path.
