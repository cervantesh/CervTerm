# ADR: Bound terminal image lifetime, transports, and resources

## Status
Accepted

## Date
2026-07-20

## Context

Kitty APC, later Sixel DCS and iTerm OSC images can carry compressed attacker-controlled payloads and create state that crosses parsing, screen/history mutation, reflow, alternate screen, renderer snapshots, GL contexts and pane lifecycle. CervTerm serializes parser/core/mux mutation on the owner thread, keeps `core.Cell` at 32 bytes, and uses detached renderer-neutral snapshots. OpenGL/GLFW is the only supported renderer.

## Decision

### Ownership and layers

1. Keep `core.Cell` text-only and unchanged. Add toolkit-neutral `internal/termimage` for resource IDs, immutable decoded RGBA resources, reservations, pending transfers and placement descriptors.
2. A process-level budget owner is created by mux. Each pane receives a child lease/store; resource and transfer IDs are pane-scoped and cannot address another pane. Pane transfer moves the store unchanged; pane close releases all CPU reservations and pending work.
3. `core.Terminal` owns only screen-adjacent placement sidecars and mutation hooks. Primary/history and alternate placements are separate; pane-wide resources may be referenced by either.
4. Render snapshots carry detached placement descriptors and stable `(PaneID, ImageID, ResourceGeneration)` references, never mutable pixels, store pointers or GPU handles. A bounded mux resource-acquisition seam returns one detached immutable byte copy only for a cache miss and revalidates generation.
5. GPU texture caches are projection/GL-context local, keyed by pane/resource identity. Visible frame references pin entries; deterministic unpinned LRU owns eviction. Pane transfer uploads in the destination and never moves GL handles.

### Parser and protocol

6. Add shared bounded control-string framing with distinct APC and DCS states. ST terminates; CAN/SUB cancel; overflow enters discard-until-terminator; EOF/reset drops the candidate. Malformed, cancelled, truncated, unknown and oversized commands commit nothing and recover to ordinary UTF-8 parsing.
7. APC/DCS framing emits bounded chunks into a protocol sink. It never reuses the 64 KiB OSC buffer and never writes payload bytes as text. Protocol replies are copied into the existing pane reply queue and written only after parser/model commit.
8. Phase 13 Kitty accepts direct data only. File, path, temporary-file and shared-memory transports are rejected. Animation/frame composition is deferred. Support is not advertised while disabled.
9. Initial supported actions are transmit, transmit-and-place, place, delete and query for bounded direct RGB/RGBA, bounded zlib raw data and PNG. Unsupported keys/actions/formats return fixed value-free errors only when the request asks for replies.

### Hard security caps

Hard caps are immutable. Configuration may only lower operational limits and is restart-scoped.

| Resource | Per pane | Process/global | Additional hard cap |
| --- | ---: | ---: | --- |
| One APC/DCS frame | 256 KiB encoded | — | discard atomically on overflow |
| Pending transfers | 8 | 32 | 10 s lifetime, max 4,096 chunks/transfer |
| Pending encoded bytes | 8 MiB | 32 MiB | reserve before append/base64 decode |
| One decoded image | 64 MiB | — | 4,096×4,096 and 16,777,216 pixels |
| Decoded CPU residency | 64 MiB | 256 MiB | checked stride/size arithmetic |
| Images | 256 | 1,024 | stable generation on ID reuse |
| Placements | 1,024 | 4,096 | max 256-cell span per axis |
| Decode workers | 1 | 2 | 250 ms acceptance deadline; late result discarded |
| Reply | 512 B | 64 KiB pending/pane | fixed codes, no payload echo |
| GPU textures/context | — | 512 entries / 256 MiB | visible pins, unpinned LRU |

Decode cannot be forcibly preempted safely in-process; byte/pixel/dimension/chunk/concurrency caps are the security boundary. The 250 ms deadline prevents late commit and reports timeout; it is not claimed as CPU preemption.

### Placement lifecycle

Placements are cell-anchored rectangles with source crop, cell span, z-order and immutable resource generation.

| Mutation | Rule |
| --- | --- |
| Explicit Kitty delete | Remove matching placements; remove resource only when selector requests and no placement retains it |
| Text erase overlapping a placement | Delete the whole placement atomically; never split/crop into fragments |
| Insert/delete chars or lines | Shift wholly-contained placements with the cell operation; delete any placement crossing the mutation boundary |
| Partial scroll region | Move wholly-contained placements; delete boundary-crossing placements |
| Full-screen upward scroll | Move placements into history with rows; history eviction releases placements |
| ED3 / clear scrollback | Delete history placements only |
| Primary reflow | Map top-left logical anchor through the existing reflow anchor mapping, preserve cell span, delete if the anchor is evicted |
| Alternate enter/exit | Preserve primary/history sidecar; use independent alternate placements; leaving alternate discards alternate placements |
| Alternate resize | Top-anchor crop/extend; delete placements whose anchor is cropped, clip visible projection only |
| RIS/reset | Cancel transfers and delete all pane placements/resources; normal screen-reset semantics otherwise remain unchanged |
| Pane close | Cancel work, invalidate generations, release all CPU state; context caches evict asynchronously on owner thread |

Z-order is bounded signed metadata: negative draws below text, zero/positive draws above text but below application overlays/cursor. Every quad is clipped by the existing pane clip.

### Atomicity, replies, and failure

- Reserve encoded/global budgets before accepting bytes; reservation rollback is exact on cancellation/failure.
- Decode into candidate immutable resources off-thread under bounded concurrency. Owner-thread generation revalidation atomically commits resource/placement and reply.
- Failure never replaces a prior resource generation, creates a placement, or emits a success reply.
- Cache/upload failure leaves model state intact, draws a deterministic placeholder/omission, and may retry under bounded backoff; it never blocks terminal input.
- Diagnostics/replies contain fixed action/error IDs and counters only, never payload bytes, decoded pixels, file names or raw metadata.

## Consequences

- Phase 13 touches VT, core, render, mux and GLFW but preserves one-way dependencies and renderer selection exclusion.
- Text-only paths pay only generation checks and nil/empty sidecar branches; benchmarks enforce a negligible regression budget.
- Sixel and iTerm adapters can target the same model in Phase 14 without changing ownership/lifecycle.
- Exact Kitty conformance is staged; default remains disabled until security, fuzz, lifecycle, rendering and performance gates pass.

## Rejected alternatives

- Put image IDs/pointers in `core.Cell`: rejected for the 32-byte invariant and pervasive copy/hash cost.
- Keep GL textures in panes/resources: rejected because panes transfer between GL contexts.
- Reuse background-layer cache as authoritative terminal state: rejected because trusted config resources and terminal-originated pane lifetimes differ.
- Full-buffer APC collection with a larger OSC cap: rejected for memory amplification and loss of streaming/cancellation semantics.
- External file/shared-memory transports: rejected for Phase 13 security scope.
- Animation: deferred pending a separate timing/resource decision.

## Rollback

Keep `graphics.kitty.enabled=false` and restart. Parser framing safely discards unsupported APC; pure model/parser pieces may remain inert. Reverse frontend cache/draw, Kitty adapter, snapshot/resource acquisition, placement sidecars and store in that order. Never change renderer selection or `core.Cell` to perform rollback.
