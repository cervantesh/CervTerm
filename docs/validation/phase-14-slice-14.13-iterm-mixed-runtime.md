# Phase 14 Slice 14.13 — Test-only iTerm and Mixed Runtime

Date: 2026-07-22
Base: `028f514`

Added programmatic/test-only iTerm mux integration to the same image runtime used by Kitty and Sixel. One control-string sink independently dispatches APC, selected DCS and selected OSC 1337. iTerm frame termination captures pane/model/store identity and epoch, fresh image/anchor/reflow generations, canonical anchor, exact metrics, internal IDs, immutable metadata and deadline. Completion revalidates every captured invariant before one create-only ephemeral resource+placement commit; success emits only `PaneDirty` and no cursor effect or reply.

All three protocols share one two-worker FIFO scheduler, one active slot per pane, process/pane stores and pending budgets. Sixel/iTerm allocate no Kitty reply slot and cannot reorder Kitty, DSR or OSC replies. The frontend and public config still do not set Sixel/iTerm options.

Added injected, panic-contained Phase 14 diagnostics with an exact privacy-safe surface: protocol, fixed reason, count and duration only. Adapter/job/submission/decode/stale/late/commit/expiry failures are classified without payload, image, pane, transfer, placement, pixel, name or base64 data.

Validation passed:

- focused iTerm/mixed/shared-scheduler tests, ten repeated runs and focused race;
- full tagged/untagged tests and vet;
- full race plus tagged focused race;
- Phase 13/14 import, maturity and diff gates;
- independent slice and concurrency/lifecycle review; the missing fixed diagnostic contract and successful late-candidate test were added and re-reviewed with Decisions/Cross-slice/Research all OK.

Coverage includes all eight protocol-option combinations, iTerm anchor/span/size/no-reply behavior, stale/reset/RIS/reflow/metrics/model/store paths, hidden/transfer/restore, deadline equality/late success rejection, rollback, close/shutdown, ephemeral retirement, cross-protocol pane exclusion/FIFO/two-worker bounds, exact 8-pane/32-process pending limits and byte-exact Kitty/DSR/OSC reply ordering.

A standalone ten-sample disabled-frame run measured median 7.487 ns/op, 0 B/op and 0 allocs/op. A contemporaneous clean-base `028f514` run on the same host measured 7.593 ns/op; the slice is 1.39% faster relative to its base. The older 7.13 ns/op artifact shifted with host conditions, so the paired clean-base comparison supplies the unexplained-regression gate while retaining zero allocations/wakes.

The mux remains owner-thread-only. Active decode ownership intentionally remains bounded until the worker returns, matching the accepted scheduler contract; reset/close immediately invalidate publication but do not prematurely release the pane slot.
