# ADR: Bound Sixel and iTerm adapters to the Phase 13 image model

## Status

Superseded by ADR 0016

## Date

2026-07-23

## Context

Phase 13 established bounded pane-local image resources and placements, owner-thread transactions, detached snapshots, process/pane budgets, asynchronous decode and context-local OpenGL caches. Phase 14 adds Sixel DCS and an iTerm OSC 1337 inline-data subset without widening `core.Cell` or adding external I/O.

## Decision

Reuse the Phase 13 store, lifecycle, scheduler, snapshots and GL cache. Add independent strict-v2, restart-scoped, default-off Sixel and iTerm flags. Accept only bounded direct terminal payloads and normalize them to a decoded candidate plus placement intent. Workers never mutate parser, core, mux, replies or GL; owner completion revalidates and commits atomically. No Phase 14 protocol emits a wire reply.

Sixel is a static DCS `q` subset with bounded raster commands, repeats and an image-local 256-color palette. iTerm is direct inline `OSC 1337;File=` data only; file/path/URL/download/write modes are rejected. Both are initially PNG/RGBA resource producers over the existing model.

## Supersession

Independent design review found cursor ordering, generated-ID collision, resources without a wire delete command and exact wire framing insufficiently specified. ADR 0016 is normative and deliberately narrows this decision.
