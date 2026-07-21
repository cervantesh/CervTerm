# Architecture Preflight — Phase 13 Bounded Image Model and Kitty Graphics

Date: 2026-07-20
Decision: **REQUIRES ADR + DESIGN + PLAN BEFORE CODE**
Risk: **High** (untrusted compressed input, large memory, parser recovery, screen/reflow lifecycle, GPU ownership)

## Trigger

Phase 13 adds APC framing, protocol decoding, pane/global resource accounting, screen-adjacent placement state, renderer-neutral image references, GL textures and terminal-originated replies. It crosses VT/core/render/mux/frontend boundaries and creates a new resource/security boundary. Renderer selection remains excluded.

## Existing seams

- `internal/vt` has bounded all-or-nothing OSC collection but no APC/DCS states; OSC’s 64 KiB accumulator is not a safe Kitty transport.
- `core.Cell` is pinned at 32 bytes and must remain text-only.
- Primary/history reflow and alternate-screen crop/restore are distinct; placements must explicitly join erase/edit/scroll/history eviction/reflow/reset lifecycle.
- Each mux pane already owns one parser, terminal and reusable snapshot and survives tab/window transfer intact.
- Render snapshots are renderer-neutral; row hashes cannot observe image-only mutations.
- GLFW/OpenGL work is OS-thread/context-owned; textures must remain projection/context-local.
- `internal/background` provides checked size math, bounded decode, leases/pins and deterministic unpinned LRU precedent, but terminal streams require stricter pane/global/time/chunk limits.

## Mandatory architecture decisions

1. Protocol-neutral resource/store/placement ownership without enlarging `Cell`.
2. APC/DCS framing, cancellation, overflow discard and normal-text recovery.
3. Immutable hard caps plus user-lowerable operational caps for every encoded/decoded/pixel/count/time/CPU/GPU stage.
4. Primary/history/alternate placement state machine for erase, insert/delete, scroll regions, eviction, resize/reflow, reset and pane close.
5. Renderer-neutral snapshot identity and safe resource acquisition without mutable pixel aliases.
6. Main-thread model commit/reply ordering versus bounded worker decode and late-result rejection.
7. Projection/context-local texture cache, visible pins, deterministic eviction, pane transfer and teardown.
8. Supported Kitty subset, fixed redacted replies and explicit rejection of file/temp/shared-memory transports.

## Stop conditions

- Any design adds image identity/pointers to `core.Cell`.
- Encoded or decoded bytes can grow without both pane and process reservation.
- A decoder can allocate from untrusted dimensions before checked bounds.
- Parser overflow/cancellation can leak payload into text or partially commit resources/placements.
- Worker or native code mutates terminal/mux/GL state off owner thread.
- Snapshots expose mutable store backing or GL handles.
- GPU resources cross GL contexts during pane transfer.
- Disabled/default paths advertise Kitty support or change text-only allocation/frame cadence.
- File/path/shared-memory transports or animation enter Phase 13 without a separate decision.

## Required gates

- Accept a concrete successor to tracked proposed image ADR-0006 with budget and lifecycle tables.
- Persist feature design and dependency-aware slice plan.
- Independently review design/plan; no external-engine challenge is invoked without explicit user approval.
- Every code/test slice runs full tests, tagged tests, race, vet, maturity, fuzz smoke and text-only performance comparison before local merge. Documentation-only Slice 13.0a runs its exact JSON/authority checks plus full, tagged, maturity and diff gates; it cannot run image fuzz/performance targets that do not exist yet.
