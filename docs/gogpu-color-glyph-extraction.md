# GoGPU color glyph extraction boundary

This note defines what should be separated from CervTerm before proposing an upstream GoGPU contribution.

## Keep in CervTerm

Terminal-specific responsibilities stay in CervTerm:

- VT/PTY parsing and screen state.
- Cell grid, wide-cell continuation, scrollback, selection, cursor rendering.
- GLFW/OpenGL texture atlas upload and drawing.
- Mapping `core.RuneWidth` to terminal cell span.
- Terminal visual policy such as cursor/selection/ANSI colors.

## Candidate GoGPU contribution

The reusable part is a renderer-neutral glyph backend, currently prototyped under `internal/fontglyph`:

```go
type Spec struct {
    Family string
    Size   float64
    DPI    float64
}

type RasterizedGlyph struct {
    Image    *image.RGBA
    Width    int
    Height   int
    BearingX int
    BearingY int
    AdvanceX float64
    CellSpan int
    HasColor bool
}

type Backend interface {
    CellMetrics() (width int, height int, baseline int)
    Rasterize(r rune, cellSpan int) (RasterizedGlyph, bool)
}
```

For GoGPU, the terminal-specific `CellSpan` parameter could be generalized to a requested canvas width or layout span. The important portable contract is:

- real font fallback,
- glyph ID/cluster-aware lookup,
- baseline/bearing metrics,
- RGBA output for color glyphs,
- `HasColor` so renderers do not foreground-tint emoji,
- cache keys by font face, glyph id/cluster, size, DPI, and color mode.

## Current CervTerm prototype state

`internal/fontglyph` is intentionally pure Go and does not import GLFW/OpenGL or CervTerm terminal packages. It currently provides:

- OpenType/Gomono primary font loading via `golang.org/x/image/font/opentype`.
- Windows system fallback face loading where available.
- Baseline/cell metrics.
- Monochrome glyph rasterization with bearing/advance metrics.
- Pure SFNT color table detection for `CBDT/CBLC`, `sbix`, `COLR/CPAL`, and `SVG`, including COLR version reporting.
- `sbix` PNG/JPEG bitmap glyph extraction and backend wiring that can return `HasColor=true` when an sbix color font is available.
- Initial `CBDT/CBLC` PNG bitmap glyph extraction for common index formats 1/2/3 and image formats 17/18/19, also wired into the backend color path.
- COLRv0/CPAL layer and palette extraction plus initial vector layer rasterization into composed RGBA glyph images.
- Initial COLRv1 paint graph support for `PaintGlyph`, `PaintSolid`, `PaintLinearGradient`, `PaintRadialGradient`, `PaintSweepGradient`, `PaintColrLayers`, `PaintColrGlyph`, `PaintComposite` for Porter-Duff modes through `PLUS`, separable blend modes through `MULTIPLY`, and HSL blend modes, non-variable transform wrappers, and `PaintVar*` parsing with ItemVariationStore-backed deltas applied to `PaintVarSolid` alpha, `PaintVarTranslate` dx/dy, transform F2DOT14/FWORD values for scale/rotate/skew families, `PaintVarLinearGradient`/`PaintVarRadialGradient`/`PaintVarSweepGradient` coordinate fields, and VarColorLine stop offset/alpha fields when normalized variation coordinates are available; unsupported paint formats fail closed and fall back instead of corrupting output.
- Initial OpenType `SVG ` document extraction by glyph ID range, wired into font loading, plus a minimal SVG rasterizer for simple `rect`, `circle`, filled `path` documents (including line, quadratic, and cubic curve flattening), solid `text`/`tspan` content with `font-size`, `text-anchor`, and `dominant-baseline` handling, and `linearGradient` fills with `opacity` and `viewBox` support.
- Optional real system color-font fixture detection (`seguiemj.ttf` on Windows, Noto Color Emoji paths on Linux) that skips when no known fixture is installed and now rasterizes both a known emoji and a representative multi-glyph emoji set through the backend when available, plus small redistributable synthetic table fixtures under `internal/fontglyph/testdata/` for SVG gradient, SVG text layout, COLRv1 PaintVarScale, and COLRv1 PaintComposite coverage, plus real TTF coverage via redistributable Go fonts from `golang.org/x/image/font/gofont` and a licensed pyftsubset-generated Noto Color Emoji subset with license/provenance files.
- Initial `RasterizeCluster` API plus GLFW renderer integration that groups combining marks, variation selectors, emoji modifiers, and ZWJ emoji sequences into one atlas glyph request instead of always drawing independent runes; canonical combining clusters now try NFC composition into a single shaped glyph before falling back to string drawing.
- A renderer-neutral `Shaper` interface now exists and has a default `SimpleShaper` pure-Go sfnt backend for simple glyph-ID/advance shaping; future HarfBuzz/DirectWrite-grade backends can replace it for complex scripts and ZWJ emoji before `RasterizeCluster` falls back to NFC/string rendering.

It does **not** yet implement the full color glyph matrix. The remaining GoGPU-sized contribution sequence is:

1. Replace the current basic SVG text face with richer font selection/real outline text and keep expanding real-glyph validation as more Windows/Linux emoji fixtures are available.
2. Add richer fixture-font subsets as coverage expands beyond the current synthetic `SVG `/COLR/CPAL table fixtures and optional system-font checks.
3. Expand cluster handling from NFC composition/simple emoji-modifier/ZWJ grouping toward full GSUB/GPOS shaping for ZWJ emoji and complex scripts.

## Why this split

The failed Edge/cmd.exe attempt proved that external screenshot-style emoji rendering lacks terminal-grade metrics and clips vertically. WezTerm, Alacritty, and Kitty all keep glyph metrics and color-glyph state inside their normal font pipeline. The GoGPU contribution should therefore be a font/color-glyph pipeline, not a terminal renderer.
