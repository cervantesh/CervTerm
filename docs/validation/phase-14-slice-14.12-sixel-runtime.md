# Phase 14 Slice 14.12 — Test-only Sixel Mux Runtime

Date: 2026-07-22
Base: `4610e8b`

Added programmatic/test-only Sixel mux integration over the existing protocol-neutral image scheduler. One control-string sink independently dispatches Kitty APC and selected Sixel DCS. At frame termination the owner thread captures pane/model/store identity and epoch, fresh image/anchor/reflow generations, canonical anchor, exact metrics, internal IDs, raster dimensions, deadline, and a detached opaque effective 256-color palette.

Sixel shares the process scheduler, two workers, queue and one-active-job-per-pane rule with Kitty. Completion revalidates registry/model membership, pointer identities, store epoch, generations, metrics, deadline, IDs, raster, span and candidate validity before one create-only `ResourceEphemeral` image commit. Success emits only `PaneDirty`; failure/staleness emits no reply, cursor effect, or partial publication. Expiry preserves an on-time buffered result and does not release scheduler activity before worker return. Shutdown clears both protocol owner graphs after closing the scheduler.

The frontend and public config do not set `SixelEnabled`; production remains unchanged/default-off.

Validation passed:

- focused Sixel/shared-scheduler tests repeated ten times and focused race tests;
- full tagged/untagged tests and vet;
- full race plus tagged focused race;
- Phase 13/14 import, maturity and diff gates;
- independent slice and concurrency/lifecycle reviews, with missing restore coverage and retained shutdown-owner graphs fixed and re-reviewed.

Coverage includes option/adapter isolation, one-sink mixed dispatch, captured history/alternate anchor and palette, exact metrics, no reply/cursor effect, stale image/anchor/reflow/metrics/store/model paths, reset/RIS, hidden panes, cross-window transfer, restore publication, deadline boundaries, submission/commit rollback, close/shutdown, shared cross-protocol exclusion, and final-placement ephemeral retirement.

Ten two-second single-CPU disabled-frame samples measured 6.858–7.560 ns/op, median 7.297 ns/op, 0 B/op and 0 allocs/op. The Phase 14 baseline median is 7.13 ns/op; the 2.34% difference remains inside the 3% gate with no new disabled-path allocation or wake.
