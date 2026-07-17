# ADR-0009: Compose local named color schemes before inline colors

- **Status:** Accepted
- **Date:** 2026-07-17
- **Gate:** Phase 3

## Context

Phase 3 adds WezTerm-style local `color_schemes` declarations and a `color_scheme` selector to the existing ADR-0002 composition pipeline. A scheme is a reusable palette base. Inserting it at the wrong point could overwrite profile/CLI `colors.*`, lose indexed-key provenance, or force renderer changes.

## Decision

1. `color_schemes` is a top-level v2 map of non-empty names to strict partial palettes: foreground, background, cursor, selection background, exact ANSI 16, and sparse indexed colors 16–255. Every declaration validates even when unselected.
2. Duplicate names merge in canonical source order. Scalars/ANSI replace; indexed entries merge numerically per key.
3. Live `color_scheme` follows include < primary < environment < profile < CLI precedence. No dedicated environment variable or flag is added.
4. Effective palette precedence is defaults < selected scheme < explicit `colors.*` from include/primary/environment/profile/CLI. Composition defers ordinary colors, resolves catalog/selector, applies the scheme once, then replays explicit colors once in existing order. Runtime patches remain above composed colors.
5. Missing selected schemes reject atomically; no selector preserves current output.
6. `LayerColorScheme` records value-free effective provenance using scheme name plus declaration source/version. Numeric paths remain `colors.indexed_colors[N]`.
7. Explain/doctor show selector and effective colors, not catalog values.
8. No core, mux, OSC, remote catalog, or renderer-backend direction changes.

## Verification

- strict validation including unselected schemes;
- selector precedence and CLI override;
- duplicate partial merge, ANSI replacement, indexed merge/unset;
- provenance defaults → scheme → explicit colors;
- failed edits preserve active colors;
- exact Shades of Purple fixture qualification.
