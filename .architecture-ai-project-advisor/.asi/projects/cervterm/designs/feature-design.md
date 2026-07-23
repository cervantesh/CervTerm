# Phase 14 Feature Design — Bounded Sixel and iTerm Inline Images

Date: 2026-07-23
Status: **Reviewed, revised and ready for sequential implementation**
Authority: ADR-0014, ADR-0016 and Phase 14 preflight/guardrails

## Intent and scope

Add independently enabled experimental static Sixel DCS and iTerm OSC 1337 direct-inline PNG adapters. Normalize both into Phase 13 resource/placement transactions, lifecycle, detached snapshots and context-local OpenGL cache.

In scope: exact bounded Sixel raster/RGB/repeat/band subset; streaming iTerm strict-base64 PNG with exact size and at most one cell dimension; cursor-neutral frame-termination anchor; internal high-half IDs; ephemeral final-placement retirement; one shared scheduler/budget; independent strict-v2 restart default-off flags.

Out of scope: C1 controls, external I/O, JPEG/GIF, HLS/palette persistence, Sixel scrolling/DECSDM, pixel/percent/two-axis/stretch sizing, animation, replies, cursor effects, renderer selection and stable support claims.

## Layers

```text
PTY -> VT selected DCS/OSC streaming framing
    -> pure sixel / itermimage adapters and sealed transfers
    -> shared bounded image workers
    -> mux owner revalidation
    -> core atomic ephemeral resource+placement transaction
    -> existing detached snapshot -> context-local GL cache/draw
```

Protocol packages depend only on `termimage` and plain standard-library values. They import no core/render/mux/frontend/config/VT package and no filesystem/network/process/unsafe facility.

## Parser contracts

### Sixel DCS

Select only 7-bit `ESC P q|0q|0;0q|0;0;0q`; ST terminates. Borrowed chunks are <=16 KiB and the whole logical frame remains <=256 KiB. CAN/SUB, overflow, reset/EOF or unsupported preamble/final enters discard and emits one cancellation; payload never reaches text.

### iTerm OSC

Probe exact 7-bit `OSC 1337;`; selected payload streams in borrowed <=16 KiB chunks and accepts BEL or ST. Retained bytes are adapter/store-owned under 8 MiB/pane, 32 MiB/process and 4,096 chunks/transfer. Existing nonselected OSC remains a 64 KiB all-or-nothing collector.

## Protocol state and syntax

### Sixel

`Idle -> Header -> RasterDeclared -> Payload -> Sealed -> Submitted`, with discard until ST on error. Require one `"1;1;W;H` before pixels. Accept `?`–`~`, `!N<char>` (`1..4096`), `#N`, `#N;2;R;G;B`, `$`, `-`. Reject HLS, second declaration, unknown forms and out-of-canvas writes. Palette is an owner-captured detached 256-color table; definitions are image-local; unset pixels transparent.

Decode uses two checked passes. Every syntax command and every expanded output column consumes one operation; combined <=4,194,304. Dimensions <=4,096, pixels <=16,777,216 and one decoded image <=64 MiB. With captured positive cell pixels `cw x ch`, placement is `Cols=ceil(W/cw)` and `Rows=ceil(H/ch)`; checked results outside `1..256` reject rather than clip. Worker returns immutable RGBA candidate plus the normalized span.

### iTerm

`Idle -> Metadata -> Base64 -> Sealed -> Submitted`, with discard until BEL/ST on error. Require exactly `inline=1`, positive decimal `size` and non-empty strict padded base64. Optionally accept exactly one `width=N` xor `height=N` (`1..256`) and absent/`1` `preserveAspectRatio`. Reject duplicates, unknowns, name, auto/pixel/percent/two-axis/stretch and external-I/O modes.

Worker uses strict base64 and the shared bounded PNG codec; decoded size must match exactly and input must contain one PNG with EOF. Let PNG pixels be `Wi x Hi` and captured cell pixels `cw x ch`. With no explicit axis, `Cols=ceil(Wi/cw)` and `Rows=ceil(Hi/ch)`. For explicit width `C`, set `Cols=C` and `Rows=ceil(Hi*C*cw/(Wi*ch))`; for explicit height `R`, set `Rows=R` and `Cols=ceil(Wi*R*ch/(Hi*cw))`. Every multiply/add/divide is checked before use; zero or results outside `1..256` reject, never clip.

## Ownership and execution

1. Owner parser streams into one pane-local enabled adapter and reserves before retention.
2. At terminator mux captures pane object/store epoch/image generation, canonical anchor, exact cell metrics, palette and internal IDs.
3. One FIFO scheduler owns all Kitty/Sixel/iTerm jobs: two workers, one outstanding job/pane, queue storage 32. Existing 8/pane and 32/process pending-transfer caps remain inclusive and authoritative.
4. Queue wait counts toward 250 ms acceptance. Expiry prevents commit but does not free pane activity/ownership before worker return and owner cleanup.
5. Worker returns candidate/span only. Owner rejects stale/late/closed/reset/metrics-changed completion and releases exactly once.
6. Owner atomically commits one create-only ephemeral resource+placement. Success dirties pane; failure changes nothing.

Workers never mutate parser/core/mux/replies/GL. GL cache/upload/delete remains projection/context-owned.

## Coordinates, IDs and lifecycle

Canonical anchor: primary row=`ScrollbackLines+CursorRow`; alternate row=`CursorRow`; column=`CursorCol`. Cursor never changes. Later PTY movement cannot change captured intent.

Kitty wire image/placement IDs use `1..0x7fffffff`; internal generated IDs use `0x80000000..0xffffffff`, monotonically without wrap/reuse for pane lifetime. Internal IDs are not Kitty-addressable.

Sixel/iTerm resources are ephemeral. Prepared lifecycle mutations delete the resource when its final placement retires through edit/erase/scroll/history eviction/ED3/reflow/alternate exit/delete/reset/close. Kitty resources remain durable. No protocol marker enters `core.Cell` or render snapshots.

## Configuration and activation

Strict v2:

```lua
graphics = {
  kitty = { enabled = false },
  sixel = { enabled = false },
  iterm = { enabled = false },
  limits = { -- existing lower-only shared limits }
}
```

All flags are independent, default false and restart-scoped. `imagesEnabled = kitty || sixel || iterm` is the sole condition for shared limits/process budget/stores/scheduler and one cache per GL context. Instantiate only enabled adapters. All-disabled remains literal nil with no allocation, deadline, draw or idle wake. Activation and restore publish old-or-new and close failures in reverse order.

## Diagnostics, tests and support

Diagnostics expose only protocol, fixed reason, counts and duration—not payload, pixels, metadata or base64. Sixel/iTerm emit no wire replies and reserve no reply slots.

Required coverage: every parser split/terminator/cancel/overflow/EOF/reset/non-leak; grammar and decode fuzz; operation/pixel/PNG bombs; namespace boundaries; ephemeral lifecycle truth table; mixed scheduler fairness/saturation/races; stale metrics/generation/deadline; config/provenance/doctor; all protocol combinations and activation fault rollback; all-disabled allocation/idle baselines; real Windows/OpenGL matrix.

Support remains experimental/default-off and `support_claim=none` if manual qualification is unrun or fails.

## Trade-offs and wrong-design conditions

Cursor neutrality avoids asynchronous reorder but narrows compatibility. High-half IDs guarantee collision freedom but restrict Kitty wire IDs. Ephemeral retirement prevents unreachable residency but adds lifecycle work. A shared scheduler enforces aggregate bounds but permits bounded contention.

Return to architecture review for external I/O, protocol-specific pools/stores/caches, delayed cursor mutation, off-owner model/GL work, cap widening, mutable snapshot aliases, payload logging, internal-ID replacement, leaked ephemeral resources, widened `core.Cell`, early active-slot release, default-on behavior or renderer changes.
