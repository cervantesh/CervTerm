# Phase 13.7 — Mux image lifecycle and bounded replies

Date: 2026-07-21
Branch: `feat/parity-phase-13-image-mux`
Base: local `dev` at `1caa3fc`

## Scope

- Optional mux-wide process image budget and one claimed store per enabled pane.
- Exact pane/generation detached resource acquisition.
- Store identity preservation through pane transfer and deterministic release through close, rollback, restore abort, and shutdown.
- Shared sequenced reply queue for existing parser replies and reserved asynchronous image replies.
- Fixed 512-byte reply frame and 64 KiB per-pane pending limits with suppression/accounting counters.
- No Kitty parser/config/render activation; default mux options remain image-free.

## Automated evidence

- `go test ./... -count=1` — pass.
- `go test -race ./internal/mux ./internal/core ./internal/termimage -count=1` — pass.
- `go run ./scripts/check-phase13-imports.go` — pass.
- `git diff --check` — pass.
- Independent final review — PASS; no blocker-level ownership, acquisition, reply-order, accounting, or default-off findings.

## Performance evidence

Candidate captures:

- `phase-13-slice-13.7-text.txt`
- `phase-13-slice-13.7-control.txt`
- `phase-13-slice-13.7-store.txt`

The text-only baseline gate passed with zero allocation regressions. Control/store comparisons against freshly captured local `dev` passed as identical-source diagnostic comparisons; their measured source hashes were unchanged by this slice, and mandatory allocation limits remained satisfied. Timing differences against older checked-in captures were treated as host noise rather than code regressions because Slice 13.7 does not modify those benchmarked VT/store sources.

## Success criteria disposition

- Default creates no image infrastructure: covered by `TestMuxImageStoresAreOptionalAndPaneLocal`.
- Pane stores are unique under one process budget: covered by bootstrap/split/restore tests.
- Transfer preserves pane, store, session, and resource identity: covered by cross-window transfer test.
- Wrong-pane/stale-generation acquisition fails and returned RGBA is detached: covered.
- Restore abort and shutdown close stores and return process usage to zero: covered.
- Existing synchronous replies retain FIFO order; async reservations prevent overtaking: covered.
- Empty, oversized, exhausted, and capacity-bound replies are suppressed without unbounded uncharged entries: covered.
- Queue bytes plus async reservations never exceed 64 KiB; every frame is at most 512 bytes: covered.

## Deferred

Strict v2 configuration, runtime activation, protocol decoders, renderer caches, and Kitty presentation remain deferred to later Phase 13 slices.
