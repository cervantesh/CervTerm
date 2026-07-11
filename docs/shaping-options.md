# Text Shaping Options for CervTerm

CervTerm currently handles Unicode width, combining marks, NFC composition, emoji modifiers, variation selectors, and ZWJ cluster grouping. That is not the same as full shaping: real shaping applies GSUB/GPOS features to turn a Unicode cluster into positioned glyph IDs.

## What full shaping must solve

- Complex scripts: Arabic joining, Indic reordering, Thai marks, Hebrew marks, etc.
- Emoji: fully-qualified ZWJ sequences, variation selectors, skin tones, and fallback when a font lacks a sequence glyph.
- Font fallback: choose one face for a whole cluster where possible, not one rune at a time.
- Metrics: return positioned glyph IDs/advances/bearings that the atlas can rasterize consistently.

## Option A: HarfBuzz via cgo

Pros:

- Best cross-platform shaping coverage.
- Mature GSUB/GPOS implementation used by browsers/toolkits.
- Works for complex scripts and emoji sequence shaping when fonts support the glyphs.

Cons:

- Introduces cgo/native library packaging complexity.
- CI/release builds need HarfBuzz development libraries or vendored binaries.
- Windows packaging must include or statically link the runtime dependency.

Best if CervTerm prioritizes correctness across platforms over pure-Go simplicity.

## Option B: Windows DirectWrite first

Pros:

- Native Windows shaping, fallback, color font, and glyph raster paths.
- Strong fit for the current Windows-first product direction.
- Can eventually replace parts of the custom font/color pipeline for Windows.

Cons:

- Windows-only; Linux/macOS need separate shaping paths later.
- Requires syscall/COM integration or a helper library.
- Moves the renderer toward platform-specific font backends.

Best if CervTerm prioritizes native Windows emoji/text quality first.

## Option C: Pure Go shaping library

Pros:

- Keeps builds simple and portable.
- Avoids cgo and native packaging.
- Better fit for a reusable Go font backend if coverage is sufficient.

Cons:

- Need to verify GSUB/GPOS and emoji coverage before depending on it.
- May lag HarfBuzz/DirectWrite behavior on tricky scripts.
- Color font integration still needs careful glyph ID handoff.

Best if dependency simplicity is more important than matching browser/native shaping exactly.

## Recommended next slice

The first slice is now implemented: CervTerm has a narrow shaping abstraction and `RasterizeCluster` tries a configured `Shaper` before falling back to the existing NFC/string path. The current interface is:

```go
type ShapedGlyph struct {
    GlyphID uint16
    XOffset float64
    YOffset float64
    XAdvance float64
}

type Shaper interface {
    Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool)
}
```

The second slice is also implemented: `SimpleShaper` is the default pure-Go backend. It maps simple clusters to sfnt glyph IDs and advances, handles NFC-composable clusters as one glyph, and refuses complex scripts/ZWJ emoji/variation-selector clusters so the existing fallback path remains safe. The next advanced backend slice should prove:

1. `e + combining acute` still works.
2. one Arabic joining sample shapes to multiple positioned glyphs rather than falling back.
3. one emoji ZWJ sequence either shapes to a supported glyph or safely falls back.
4. atlas upload remains isolated from shaping engine details.

## Current recommendation

For CervTerm's Windows-first beta, **DirectWrite first is now the selected path**. The repository has a Windows-only `DirectWriteShaper` entrypoint selected when `IDWriteFactory` and `IDWriteTextAnalyzer` can be created, preserves `SimpleShaper` fallback behavior, records fallback font source paths, can create `IDWriteFontFace` objects from those paths, and has a typed `IDWriteTextAnalyzer` vtable with verified `AnalyzeScript`, `GetGlyphs`, and `GetGlyphPlacements` method pointers, and implements/verifies minimal text-analysis source/sink callbacks for `AnalyzeScript`. The `GetGlyphs`/`GetGlyphPlacements` wrapper is now active: the missing COM `this` argument and `DWRITE_SCRIPT_ANALYSIS` layout were corrected, the wrapper maps DirectWrite glyph IDs/advances/offsets into `ShapedGlyph`, and tests verify simple DirectWrite shaping plus source-backed Arabic, Indic, Arabic ligature, Indic conjunct, and Segoe UI Emoji ZWJ shaping through `DirectWriteShaper`.

For a cross-platform terminal core: **HarfBuzz via cgo** is the most standards-aligned path, but it should wait until packaging is ready for native dependencies.

For the next implementation after this scaffolding, broaden scripted fixture samples beyond the current Arabic/Indic ligature/conjunct and emoji smoke cases and broaden real-world shaped color fixture samples beyond the current single-glyph and Noto subset multi-glyph COLR paths.
