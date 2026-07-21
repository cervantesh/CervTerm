# Phase 13 Feature Design — Bounded Image Model and Kitty Graphics

Date: 2026-07-20
Status: **Reviewed and revised; ready for implementation planning**
Authority: accepted ADR 0014, Phase 13 preflight/context/guardrails

## Objectives and exclusions

Add protocol-neutral bounded image resources/placements and a default-off direct-data Kitty APC subset. Preserve 32-byte text-only `core.Cell`, pane identity, primary/alternate semantics, detached snapshots, owner-thread model/GL mutation and OpenGL-only renderer direction.

Excluded: renderer selection, Sixel, iTerm, animation, external file/temp/shared-memory transport, persistent image cache and default-on activation.

## Layering

```text
PTY -> bounded vt APC/DCS framer -> Kitty adapter/candidates
    -> bounded CPU decode workers -> owner-thread core image transaction
    -> termimage pane store + core placement sidecars
    -> detached render/mux refs -> projection/context GL cache -> pane-clipped draw
```

`internal/termimage` is toolkit-neutral and imports no core/render/mux/frontend package. `core` may import it; GLFW/OpenGL remains frontend-only.

## 1. Parser framing contract

```go
type ControlStringKind uint8 // APC or DCS
type ControlStringEvent struct {
    Kind ControlStringKind
    Chunk []byte // borrowed until callback returns
    Final, Cancelled, Overflow bool
}
type ControlStringSink func(ControlStringEvent)
func (p *Parser) SetControlStringSink(ControlStringSink)
func (p *Parser) Reset()
func (p *Parser) EndOfInput()
```

Dedicated APC/DCS/escape/discard states use a 256 KiB frame budget and <=16 KiB reusable chunks. ST finalizes; CAN/SUB cancel; overflow discards through ST/CAN/SUB. `Reset` and `EndOfInput` emit one cancellation outcome for an open candidate then restore ground state. ESC not followed by `\` remains payload while under bounds. Nil sink discards. OSC behavior is unchanged; APC/DCS payload never reaches cells.

## 2. Protocol-neutral types, coordinates and store

```go
type ResourceRef struct { Image ImageID; Generation ResourceGeneration }
type PixelRect struct { X, Y, Width, Height uint32 }
type CellAnchor struct { Row int64; Col uint32 }
type Placement struct {
    ID PlacementID; Resource ResourceRef
    Anchor CellAnchor; Cols, Rows uint16
    Crop *PixelRect; Z int16; Opacity uint8
}
type DeleteSelector struct {
    Placement *PlacementID; Image *ImageID
    All, CurrentScreen, UnderCursor, DeleteResource bool
}
type Projection struct { Placements []Placement; Generation uint64 }
```

- Primary `Anchor.Row` is a bounded physical row index in `[oldest history, screen bottom]`, with zero at the current oldest retained row. History eviction shifts surviving anchors down; bounded placement count makes this deterministic.
- Alternate `Anchor.Row` is top-relative `[0, rows)`. Primary/history and alternate placement sets are independent.
- `Col`, `Cols`, `Rows` are terminal cell units; spans must be 1..256. A nil `Crop` means the complete resource. A non-nil crop must have non-zero width/height and checked `X+Width <= resource.Width`, `Y+Height <= resource.Height`; invalid/overflowing crops reject the whole transaction. Projection clips to viewport/pane but never mutates placement.
- A selector must choose exactly one addressing mode: `All`, `Placement`, `Image`, or `UnderCursor`. `CurrentScreen` is an optional scope modifier for `All`, `Image`, or `UnderCursor`; it is invalid with globally unique `Placement`. Without it, selectors inspect both primary/history and alternate sets. `UnderCursor` matches rectangles containing the active screen cursor. `DeleteResource` first selects/removes all placements referencing matched resources and then removes those resources; without it only placements are removed. Nil, contradictory, cross-pane, or unsupported selectors reject atomically.

```go
type ProcessBudget struct { /* checked atomic counters */ }
type Store struct { /* owner-thread maps, epoch, pending candidates */ }
type CandidateTransfer struct { /* reservation/epoch owner; exactly-once close */ }
type DecodedCandidate struct { /* immutable candidate + leases, not published */ }
func NewStore(*ProcessBudget, Limits) *Store
func (s *Store) BeginTransfer(Header) (*CandidateTransfer, error)
func (s *Store) Acquire(ResourceRef) (DetachedResource, bool)
func (s *Store) Reset()              // increment epoch; cancel transfers; clear resources
func (s *Store) Close()              // Reset plus permanently reject operations
```

`DetachedResource` owns a fresh RGBA copy and exposes no store alias. Generation increments on image-ID reuse. Hard caps and lower operational limits are ADR 0014 normative.

## 3. Atomic resource-plus-placement publication

Separate public `CommitDecoded` and `PlaceImage` calls are forbidden. `core.Terminal` owns the cross-store/screen transaction:

```go
type ImageCommit struct {
    Candidate termimage.DecodedCandidate
    Placement *termimage.PlacementSpec // nil for transmit-only
}
type ImageCommitResult struct { Resource termimage.ResourceRef; Placement *termimage.PlacementID }
func (t *Terminal) CommitImage(ImageCommit) (ImageCommitResult, error)
func (t *Terminal) DeleteImages(termimage.DeleteSelector) int
func (t *Terminal) ResetImages()
func (t *Terminal) ImageProjection(viewportTop, rows int) termimage.Projection
```

Transaction sequence on the owner thread:

1. Revalidate candidate store, epoch, deadline, decoded lease, dimensions and replacement generation.
2. Resolve/validate the optional placement, selector-independent limits and prior replacement state without publication.
3. Build a private `PreparedImageCommit` containing complete replacement store state and complete replacement screen-sidecar slices. All maps/slices and accounting deltas allocate and validate here; preparation failure closes the candidate and leaves old state untouched.
4. Publish by swapping the prepared store-state pointer and prepared sidecar pointer, then incrementing image generation and consuming the candidate. These swaps are infallible after preparation, execute in one owner-thread call with no callback/event/reply/read interleaving, and have no post-swap failure path.
5. Queue success reply and `PaneDirty` only after both pointers are installed. Logical readers therefore observe either the old pair or the new pair, never a partial pair.

Any error occurs before step 4 and cannot replace a prior resource generation or create/delete a placement. `ResetImages` uses the same prepared-state swap to invoke `Store.Reset` and clear both screen sidecars without closing the terminal; pane close invokes `Store.Close`.

## 4. Placement lifecycle

ADR 0014 is normative:

- Erase/overwrite overlap deletes the whole placement.
- Insert/delete char/line and partial scroll shift wholly-contained placements and delete boundary-crossing placements.
- Full-screen upward scroll moves placements into history; ring eviction and scrollback reduction delete evicted placements; ED3 deletes history placements only.
- Reflow first extracts an explicit private `reflowMap` from the existing old/new physical cell stream. Text cursor/boundary mapping and each placement top-left use that same mapping. Cell span is preserved; evicted anchors delete the placement. This is a new refactor, not a claimed existing API.
- Alternate entry preserves primary; alternate exit discards alternate placements. Alternate resize is top-anchored and deletes cropped anchors.
- RIS calls parser cancellation plus `ResetImages`; pane close cancels work and closes store.

Resource deletion occurs only on an explicit resource selector or when no placement/candidate retains it and store policy evicts it. Mutation helpers are centralized and independently truth-table tested.

## 5. Decode scheduler and replies

A mux-owned scheduler has a bounded queue of 32 sealed candidates, at most two active workers process-wide and one active worker per pane. Final-chunk seal time starts the 250 ms acceptance deadline. Workers continue to completion if Go decoding cannot be preempted, but late results cannot commit; while both slots remain occupied new work is rejected rather than spawning. Pane close/reset invalidates epoch and queued/active outcomes.

Workers perform base64, checked raw RGB/RGBA, bounded zlib and PNG decode only. They never mutate parser/core/mux/session/GL. Every allocation is preceded by dimension/stride/pixel/decoded lease checks.

All terminal replies, including existing parser/core replies and image replies, must enter one pane-owned `queueReply` helper. It accounts the shared live pending queue at 64 KiB per pane and deducts bytes as drain writes consume entries; direct `pendingReplies` appends are forbidden. Image replies additionally cap each frame at 512 B. If capacity is unavailable, the fixed image reply is suppressed and a counter increments; model state is not rolled back. Each finalized image request yields at most one fixed value-free outcome, quiet policy permitting. Success follows owner-thread commit; failure follows final classification. Payload/header values are never logged or echoed.

## 6. Render/mux projection and acquisition

```go
type ImagePlacement struct { PaneObject uint64; Placement termimage.Placement }
type Snapshot struct { /* existing */ Images []ImagePlacement; ImageGeneration uint64 }
func (m *Mux) AcquireImageResource(PaneID, termimage.ResourceRef) (termimage.DetachedResource, bool)
```

Snapshot refs contain no RGBA/store/GL alias. Reusable internal capture capacity is deep-detached by `Mux.PaneView`. `AcquireImageResource` validates pane and exact generation and copies only on projection cache miss. A removed resource makes stale snapshots skip drawing safely. Row hashes stay text-only; image generation is independent damage identity.

## 7. Configuration and activation

Strict v2 restart-scoped fields:

```lua
graphics = {
  kitty = { enabled = false },
  limits = {
    encoded_bytes_per_pane = 8388608,
    decoded_bytes_per_pane = 67108864,
    image_count_per_pane = 256,
    placement_count_per_pane = 1024,
    gpu_bytes_per_context = 268435456,
  },
}
```

Config may only lower hard caps. Includes/profiles/unset/provenance/diff/template/Teal/doctor must be complete before activation. Default/v1/disabled maps to literal nil capability: no budget/store/sink/scheduler/cache/advertisement/idle work.

## 8. Kitty subset

APC prefix `G`; direct-data only. Actions: transmit, transmit+place, place, delete and query. Formats: RGB24, RGBA32, zlib raw and PNG. Limits: 8 pending/pane, 32 process, 4,096 chunks/transfer, 10 s transfer lifetime. Unsupported keys/actions/formats/transports are atomically rejected. No animation, Unicode placeholders or filesystem access.

## 9. GPU capability, cache and teardown

```go
type ImageTexture interface { Close() error }
type TerminalImageRenderer interface {
    PrepareTerminalImage(ImageTextureKey, termimage.DetachedResource) (ImageTexture, error)
    DrawTerminalImage(ImageTexture, ImageRect, ImageRect, float32) error
}
```

This is optional; only `glRenderer` implements it. It does not change renderer selection or the base `Renderer` contract.

Each projection/GL context owns one deterministic LRU capped at 512 entries and 256 MiB (or lower config). At frame start, unique visible refs are ordered by `(z, placement render order, pane object, resource ref)`. The cache selects the longest prefix fitting both caps, pins it for that frame, and deterministically omits the rest with a counter; visible pins can therefore never exceed a cap. Prior-frame pins are released before selection. Cache misses use mux detached acquisition, then upload with the owning context current.

`ImageTexture.Close` must run with its owning projection context current. Cache teardown is registered in the existing reverse-order projection bundle before renderer destruction; both normal teardown and failure rollback explicitly make the host context current (matching live projection teardown) before cache close. GL handles never transfer with panes.

Draw order: pane background; negative-z images; text; zero/positive-z images; cursor/preedit/application overlays; pane chrome. Existing pane clip applies to every image quad. Upload failure leaves model state intact and uses bounded retry; missing textures are deterministically omitted.

## 10. Damage and performance

Image generation changes damage the affected pane (initial conservative implementation), never the whole window. Upload completion damages only referencing placements. Text-only capture/draw has an empty branch, zero image allocation, unchanged row hashes and no extra idle frames.

Acceptance gates include all focused/full/tagged/race/vet/maturity/diff checks from Phase 13 guardrails, parser/adapter/decoder fuzz, touched-file line-count checks, and repeatable baseline/candidate `benchstat`. `core.Cell` must remain 32 bytes; disabled steady-state gets zero image allocation, no idle cadence increase and <=3% median benchmark regression.

## 11. Rollback

Set `graphics.kitty.enabled=false` and restart. Code rollback is reverse dependency: activation, draw/damage, cache/capability, async integration/decoder/adapter, mux projection/acquisition, core lifecycle, store. Generic APC/DCS framing may remain only if independently fuzzed and performance-qualified.
