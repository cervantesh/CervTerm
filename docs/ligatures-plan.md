# Programming Ligatures Plan

## Where we are

The shaping stack (docs/shaping-options.md) already runs: `DirectWriteShaper`
(GSUB/GPOS via IDWriteTextAnalyzer, tested on Arabic/Indic/ZWJ) with
`SimpleShaper` fallback, feeding `RasterizeCluster` and the shared atlas. But
it only shapes single grapheme clusters — `collectRenderCluster` gates on
`ShouldShapeRune`/combining marks. Programming ligatures (`->`, `=>`, `!=`,
`===`, `<=`, `::`, `...`) live in fonts like Fira Code / Cascadia Code /
JetBrains Mono as GSUB substitutions across *multiple ASCII cells*, which the
per-cluster path never sees.

## Goal

When the configured font has ligature substitutions and
`font.ligatures = true` (default **false** in v1 — opt-in until proven), a
run of symbol characters on one row renders as the font's ligature glyph(s),
spanning exactly the run's cells. Everything logical (grid, selection, copy
text, cursor addressing) is untouched — this is render-only.

## Design

### 1. Run detection (frontend, per row)

During the row walk, detect maximal **ligature-candidate runs**: consecutive
cells where:
- rune is in the candidate alphabet: `! # $ % & * + - . / : ; < = > ? @ \ ^ _ | ~` (the
  chars participating in programming ligatures; letters/digits excluded on
  purpose — keeps runs short and avoids shaping ordinary text),
- identical render attrs (FG, BG, bold/italic/dim/inverse/underline...),
- no wide/continuation/combining cells, not the cursor's cell (see §4),
- run length 2..8 (longest real ligatures are ~4; 8 bounds cost).

Single-cell candidates fall through to the existing per-rune path.

### 2. Shaping + caching (fontglyph)

New entrypoint `RasterizeRun(run string, cellSpan int, ...)`:
- Shape `run` via the existing `Shaper` interface (DirectWriteShaper already
  handles GSUB; SimpleShaper returns !ok for multi-glyph substitution — safe
  fallback).
- If the shaper output is **one glyph per input char with default advances**
  (no substitution happened — font has no ligature for this run), report
  "no ligature" so the frontend uses the normal per-cell path AND caches that
  negative result.
- If substitution happened, rasterize the shaped glyphs into a single
  cellSpan-wide bitmap (reusing the cluster raster path) and upload to the
  atlas keyed by `(run, attrs-class, face, ppem)`.
- Cache both positive and negative results in the existing atlas-side maps —
  the run alphabet and length bound keep key cardinality small (typical
  sessions see dozens of distinct runs, not thousands).

### 3. Draw path

In the row walk, when a candidate run is found: one atlas lookup; on
ligature-hit draw the span exactly like `drawCluster` does today (span ×
cellW) and mark the covered cells via the existing `skippedGlyph` scratch;
on miss draw cells individually. Bold doubling and decorations apply over
the whole span (same as clusters).

### 4. Cursor and selection inside a ligature

- **Cursor**: if the cursor sits on any cell of a candidate run, the run is
  NOT ligated this frame (split at the cursor cell). The user always sees the
  exact character under the cursor. This is the Kitty behavior and avoids
  block-cursor-over-half-a-glyph artifacts. Cheap: cursor position is known
  before the row walk.
- **Selection**: selection highlight is a background fill per cell and stays
  per-cell (the ligature glyph draws over a partially-highlighted background;
  all terminals accept this). Copy text is untouched (logical cells).

### 5. Config

`font.ligatures = false` default. Template comment: needs a font with
programming ligatures (Fira Code, Cascadia Code, JetBrains Mono); no effect
with fonts lacking them (e.g. default Go Mono). Validation: none needed
(bool). Also gate at runtime: if the active shaper is SimpleShaper, the
feature quietly stays off (no per-frame probing).

### 6. Interaction with row damage

Ligature decisions are pure functions of row content + cursor position; the
row-damage hashes already include cells, and cursor rows are always damaged,
so a cursor entering/leaving a run correctly re-renders the row. No extra
coupling.

## Files

| File | Change |
|---|---|
| `internal/fontglyph/runshape.go` (+test) | new: RasterizeRun, no-substitution detection, negative cache |
| `internal/frontend/glfwgl/cluster.go` or new `runs.go` | candidate-run detection (pure, testable: alphabet, attr equality, cursor split, bounds) |
| `internal/frontend/glfwgl/app_draw.go` / `app_row.go` | run lookup in the row walk |
| `internal/frontend/glfwgl/atlas*.go` | run-keyed atlas entries (mirror cluster entries) |
| `internal/config/*` | `font.ligatures` bool |
| `README.md` + `docs/shaping-options.md` | note the run-shaping slice |

## Correctness traps

1. **No-substitution detection** must compare glyph count AND glyph IDs
   against per-char shaping — some fonts kern symbol pairs (GPOS only);
   advance-only changes must count as "no ligature" (we must not break the
   monospace grid for kerning).
2. Shaped span must be **forced to cellSpan × cellW** advance on the grid
   regardless of what DirectWrite returns — the grid always wins.
3. Attr equality for runs must include everything that changes rendering
   (inverse swaps FG/BG per cell — exclude inverse-mixed runs).
4. Cursor split must consider the cursor's *visual* cell (BiDi off in v1 —
   if `render.bidi` is on, disable ligatures for reordered rows).
5. Cache keys must include ppem/face/bold-italic (a bold `->` differs).
6. Negative-cache invalidation on font/scale rebuild (content-scale change
   rebuilds the atlas — run caches must rebuild with it).
7. Fallback safety: any shaper error → per-cell path, never a blank span.

## E2E verify

1. With Cascadia Code (`font.family = "Cascadia Code"`, ships with Windows) +
   `font.ligatures = true`: `echo "a -> b != c >= d"` renders ligated arrows
   (screenshot; compare against ligatures=false).
2. Cursor moved onto `->` splits it into `-` `>` while there; leaving re-ligates.
3. Default Go Mono + ligatures=true: zero visual change, zero errors.
4. Selection across a ligature highlights per-cell and copies exact chars.
5. Throughput unchanged on the 20k-line dump (negative cache does its job).
6. Full gates + maturity.

## Flow

Branch `feat/ligatures` stacked on `feat/row-damage` (both touch the row
walk; sequential avoids conflicts). Implementer: **Opus**. Reviewer:
**Fable** (traps checklist) + E2E + PR.
