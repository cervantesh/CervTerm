# ADR: Extract controllers and font subsystems behind compatibility facades

## Status

Accepted

## Date

2026-07-23

## Relationship

Extends ADR-0001's font/cluster direction and constrains implementation movement without changing font selection, fallback, shaping, raster or native behavior.

## Context

`App`, `Mux` and `fontglyph` concentrate unrelated responsibilities. The developer requires these areas to be addressed first, but ownership and behavior defects must not be changed during movement.

## Decision

App and Mux begin with private same-package delegation under unchanged entry points. Preparatory slices cannot transfer final ownership, add exported bypass ports, copy mutable state, retain `*App`/`*Mux` as a service locator, or close L1-01/L3-01. Formal thin-facade closure occurs only after their semantic dependencies merge.

For fonts, root `internal/fontglyph` remains the compatibility/orchestration facade. The exact acyclic DAG is:

```text
internal/fontglyph (compatibility/orchestration facade)
  -> discovery, cache, shape, raster, platform
  -> internal/fontglyph/internal/face
discovery -> fontdesc
cache     -> internal/face
shape     -> internal/face, fontdesc, unicodecluster, unicodeprops
raster    -> internal/face, fontdesc, unicodecluster, unicodeprops
platform  -> internal/face, fontdesc, unicodecluster, unicodeprops
internal/face -> fontdesc
```

The listed edges are the complete allowlist: public subsystem packages (`discovery`, `cache`, `shape`, `raster`, `platform`) never import the root facade or one another; they may import the private `internal/face` leaf and listed stable leaves exactly as enumerated. `fontdesc`, `unicodecluster`, and `unicodeprops` are stable leaves. The root facade coordinates cross-subsystem calls so `raster` never imports `platform` and `face` never imports a creator, preventing native-handle cycles. The face owner owns parsed SFNT/color/native handles supplied by root/platform construction. A cache lease owns exactly one pin, closes idempotently and cannot outlive its owner. Platform/native and raster resources unwind in reverse acquisition order.

Every extraction uses distinct characterization, additive seam, mechanical move, wiring and guard commits, a changed-path allowlist, package-cycle gate and allocation/performance comparison.

Controller ports are consumer-defined and concern-specific: normally no more than five methods; no `any`, `map[string]any`, generic callback bag, concrete facade return, or method set spanning two controller concerns. Controllers retain only concern-owned state, stable IDs and detached immutable snapshots. An exception requires an ADR and static allowlist update.

## Consequences

Early App/Mux work is deliberately partial. Font package extraction can complete before other findings because ownership and import direction are fixed by this ADR.

## Rejected alternatives

A big-bang rewrite, feature changes during movement, broad public interfaces, a generic service locator, or subpackages importing the facade.

## Rollback

Revert cleanup, wiring, movement and additive seam in reverse order; retain characterization tests and restore the compatibility facade as sole path.
