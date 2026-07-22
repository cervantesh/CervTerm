# ADR: Refine Phase 14 framing, cursor, and ephemeral resource semantics

## Status

Accepted; supersedes ADR 0015 where stricter

## Date

2026-07-23

## Context

Sixel and iTerm have different framing and cursor conventions from Kitty. Their asynchronous decode cannot apply delayed cursor effects without reordering later PTY input. They carry no reusable resource ID/delete command, so generated IDs can collide with Kitty and placement retirement can leave unreachable decoded resources resident.

## Decision

### Shared invariants

- Reuse the ADR-0014 pane store, process/pane budgets, owner-thread prepared transaction, screen lifecycle, detached snapshots and context-local GL cache.
- Keep `core.Cell` text-only and exactly 32 bytes. Do not add renderer selection.
- Generalize one cross-protocol scheduler: FIFO, two workers process-wide, one outstanding job per pane, queue storage 32. Existing pending-transfer hard caps remain authoritative at 8/pane and 32/process inclusive queued/running work.
- Queue time counts toward the 250 ms acceptance deadline. Expiry makes a result ineligible but does not free pane activity or ownership until worker return and owner cleanup.
- Workers return immutable candidates only. Owner completion revalidates pane/store epoch/model membership/deadline/exact metrics/internal IDs before publication.
- Independent strict-v2, restart-scoped, default-off `graphics.sixel.enabled` and `graphics.iterm.enabled` flags. Shared model/cache infrastructure exists when any image protocol is enabled and remains literal nil when all are disabled.

### Cursor-neutral placement

Phase 14 is deliberately cursor-neutral. The owner captures the anchor at frame termination; success/failure never moves the text cursor or applies delayed scrolling. Primary row is `ScrollbackLines + CursorRow`; alternate row is `CursorRow`; column is `CursorCol`. DECSDM/Sixel scrolling and iTerm cursor movement are deferred.

### Internal IDs and retention

The high half of `ImageID` and `PlacementID` (`0x80000000..0xffffffff`) is an internal monotonic namespace. Kitty wire IDs are restricted to `1..0x7fffffff`. Internal IDs never wrap/reuse while the pane lives and are not wire-addressable by Kitty.

Sixel/iTerm commit one create-only resource plus one placement as ephemeral. When its final placement retires through edit/erase/scroll/history eviction/reflow/alternate exit/delete/reset/close, the owner-thread prepared transaction also retires the resource and releases reservations exactly once. Kitty resources remain durable until explicit resource deletion/reset/close.

### Exact Sixel subset

- Accept 7-bit `ESC P q`, `ESC P 0q`, `ESC P 0;0q`, or `ESC P 0;0;0q`, terminated only by ST. C1 DCS is excluded.
- Require exactly one `"1;1;W;H` before pixel output. Width/height must be positive and within ADR-0014 dimension/pixel/decoded bounds. Drawing outside the declared canvas rejects atomically.
- Support `?`–`~`, `!N<char>` (`1..4096`), `#N`, `#N;2;R;G;B` (register `0..255`, RGB percentages `0..100`), `$`, and `-`. Reject HLS, second declarations and unknown forms.
- Count each syntax command and each expanded output column; combined work is at most 4,194,304 operations. Whole frame remains at the 256 KiB control-string bound.
- Palette is image-local and seeded from a detached effective 256-color pane palette. Unset pixels are transparent.

### Exact iTerm subset

- Accept only 7-bit `OSC 1337;File=...:<strict-base64>` terminated by BEL or ST. C1 OSC is excluded.
- Require exactly `inline=1`, positive decimal `size`, non-empty strict padded base64 and one PNG with exact decoded size/no trailing bytes.
- Optionally accept exactly one positive cell dimension (`width=N` xor `height=N`, `1..256`) and absent/`1` `preserveAspectRatio`. Derive the other axis with checked captured cell-aspect math. Reject `auto`, pixels, percentages, two axes and stretch.
- Reject duplicate/unknown fields, `name`, `inline=0`, omitted inline intent, file/path/URL/download/multipart/write forms and every external I/O mode.
- Selected OSC chunks are <=16 KiB; retained data uses existing 8 MiB/pane, 32 MiB/process and 4,096 chunks/transfer caps. Nonselected OSC retains its 64 KiB collector.

### Recovery and replies

CAN/SUB, malformed input, overflow, reset and EOF discard through the correct terminator, publish nothing and never leak payload as text. Sixel/iTerm reserve no reply slot and emit no invented reply. Kitty reply ordering/content remains unchanged.

## Consequences

The initial subsets intentionally reject forms accepted by broader implementations. Kitty loses the high-half wire-ID range. Core/store lifecycle gains protocol-neutral ephemeral retention but cells, snapshots and GL ownership do not change. Support remains experimental/default-off with no stable claim until automated and real Windows/OpenGL qualification pass.

## Rejected alternatives

Full-buffer enlarged OSC, external file transfer, delayed cursor mutation, protocol-specific stores/workers/caches, image IDs in cells and renderer selection are rejected.

## Rollback

Disable either protocol independently and restart. Code rollback proceeds activation, config, mux routing, workers/adapters/shared codec, scheduler, parser transports, ephemeral lifecycle, ID partition, anchor helper. Phase 13 Kitty behavior remains available throughout.
