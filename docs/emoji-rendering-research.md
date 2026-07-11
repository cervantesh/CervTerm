# Emoji and color glyph rendering research

CervTerm's temporary Edge/PowerShell emoji rasterization path is not the right long-term design. Established terminals render emoji through the same font pipeline as other glyphs: font fallback, shaping/metrics, glyph rasterization, atlas upload, and a color-aware shader path.

## Evidence from established terminals

### WezTerm

WezTerm bundles real fallback fonts, including Noto Color Emoji, instead of drawing emoji manually. Its fallback chain explicitly appends `Noto Color Emoji` and `Symbols Nerd Font Mono` after the configured/default font ([source](https://github.com/wez/wezterm/blob/fff02ca501c3b457f99b467a86061d2b150c51f2/config/src/font.rs#L585-L604)).

WezTerm's glyph object carries real raster metrics plus a `has_color` flag ([source](https://github.com/wez/wezterm/blob/fff02ca501c3b457f99b467a86061d2b150c51f2/wezterm-font/src/rasterizer/mod.rs#L16-L27)). Its FreeType rasterizer handles BGRA color glyph bitmaps, crops to non-transparent bounds, swaps channel order, and preserves bearing information ([source](https://github.com/wez/wezterm/blob/fff02ca501c3b457f99b467a86061d2b150c51f2/wezterm-font/src/rasterizer/freetype.rs#L317-L378)).

WezTerm also scales fallback/bitmap emoji against terminal cell metrics rather than fitting arbitrary images into cells. It computes max pixel width from Unicode cell width and base font cell width, and has special handling for bitmap emoji strikes such as older Noto Color Emoji ([source](https://github.com/wez/wezterm/blob/fff02ca501c3b457f99b467a86061d2b150c51f2/wezterm-gui/src/glyphcache.rs#L722-L803)). In the shader, color glyph textures are sampled as full color instead of being tinted by foreground color ([source](https://github.com/wez/wezterm/blob/fff02ca501c3b457f99b467a86061d2b150c51f2/wezterm-gui/src/glyph-frag.glsl#L128-L132)).

### Alacritty

Alacritty's atlas uses RGBA textures for both normal and emoji glyphs, explicitly because that supports emoji with no performance impact ([source](https://github.com/alacritty/alacritty/blob/bdb72b32eeb074e3a0b8559d8ccac458237474a3/alacritty/src/renderer/text/atlas.rs#L72-L99)). When inserting a rasterized glyph, Alacritty preserves glyph `top`, `left`, `width`, `height`, UVs, and whether the glyph is multicolor ([source](https://github.com/alacritty/alacritty/blob/bdb72b32eeb074e3a0b8559d8ccac458237474a3/alacritty/src/renderer/text/atlas.rs#L147-L221)).

During drawing, Alacritty positions a glyph using `glyph.left` and `glyph.top` relative to the cell, and expands the quad width to two cells when the terminal cell carries `WIDE_CHAR` ([source](https://github.com/alacritty/alacritty/blob/bdb72b32eeb074e3a0b8559d8ccac458237474a3/alacritty/src/renderer/text/gles2.rs#L275-L329)). Its glyph cache adjusts `top` by glyph offset and font descent before atlas upload, which is the baseline-aware step CervTerm is currently missing ([source](https://github.com/alacritty/alacritty/blob/bdb72b32eeb074e3a0b8559d8ccac458237474a3/alacritty/src/renderer/text/glyph_cache.rs#L247-L268)). Its fragment shader has a dedicated color-glyph branch for emojis ([source](https://github.com/alacritty/alacritty/blob/bdb72b32eeb074e3a0b8559d8ccac458237474a3/alacritty/res/glsl3/text.f.glsl#L51-L67)).

### Kitty

Kitty chooses AppleColorEmoji for emoji presentation on CoreText platforms before falling back to a substitute face ([source](https://github.com/kovidgoyal/kitty/blob/5879199d4af6d549aff54e1e008fe0e14f891f51/kitty/core_text.m#L431-L468)). It computes terminal font cell metrics from real CoreText layout: cell width from glyph advances, cell height from line origins, and baseline from line bounds ([source](https://github.com/kovidgoyal/kitty/blob/5879199d4af6d549aff54e1e008fe0e14f891f51/kitty/core_text.m#L595-L647)).

For color glyphs, Kitty renders into a canvas of `cell_width * num_cells` by `cell_height`, then places the glyph using the baseline (`height - baseline`) rather than using HTML/CSS font sizing ([source](https://github.com/kovidgoyal/kitty/blob/5879199d4af6d549aff54e1e008fe0e14f891f51/kitty/core_text.m#L725-L749), [source](https://github.com/kovidgoyal/kitty/blob/5879199d4af6d549aff54e1e008fe0e14f891f51/kitty/core_text.m#L1042-L1090)). Its shader also treats colored sprites as intrinsic color instead of foreground-tinted masks ([source](https://github.com/kovidgoyal/kitty/blob/5879199d4af6d549aff54e1e008fe0e14f891f51/kitty/cell_fragment.glsl#L61-L65)).

### Go ecosystem note

A pure-Go candidate, `gogpu/gg`, exposes a color font interface for CBDT/CBLC, sbix, COLR/CPAL, and SVG detection ([source](https://github.com/gogpu/gg/blob/f0b4f548e138b7b8d72a019e4f342549f0f73808/text/color_font.go#L7-L29)). It has CBDT/CBLC extraction structures with bitmap metrics ([source](https://github.com/gogpu/gg/blob/f0b4f548e138b7b8d72a019e4f342549f0f73808/text/emoji/cbdt_extractor.go#L77-L191)), and bitmap emoji drawing uses glyph `OriginX`/`OriginY` relative to baseline ([source](https://github.com/gogpu/gg/blob/f0b4f548e138b7b8d72a019e4f342549f0f73808/text/draw_emoji.go#L69-L112)). However, its own COLR path is still marked future/TODO ([source](https://github.com/gogpu/gg/blob/f0b4f548e138b7b8d72a019e4f342549f0f73808/text/draw_emoji.go#L13-L18), [source](https://github.com/gogpu/gg/blob/f0b4f548e138b7b8d72a019e4f342549f0f73808/text/draw_emoji.go#L55-L58)), so it is not a complete drop-in terminal font stack today.

## CervTerm direction

Do not continue with the Edge/PowerShell screenshot-style rasterizer as the real solution. It does not provide terminal-grade glyph metrics, baseline, shaping, fallback, or stable performance.

Instead, implement a font backend with this contract:

```go
type RasterizedGlyph struct {
    Pixels []byte // RGBA for color glyphs, alpha mask or RGBA for monochrome glyphs
    Width int
    Height int
    BearingX int
    BearingY int // top/baseline metric, not CSS box position
    AdvanceX float64
    CellSpan int
    HasColor bool
}
```

Then the renderer should:

1. Shape/select the glyph from a fallback chain: primary monospace -> CJK/system fallback -> emoji font -> symbols font.
2. Rasterize through a real font renderer:
   - Windows near-term: DirectWrite/Direct2D or FreeType with color glyph support.
   - Cross-platform long-term: FreeType + HarfBuzz, with COLR/CPAL and CBDT/CBLC handling.
3. Use the rasterizer's bearing/top/baseline metrics to position the quad.
4. Use `CellSpan`/Unicode width to size the terminal cell quad, but do not stretch a clipped image to fit.
5. Upload RGBA color glyphs to the atlas and render them without foreground tint.
6. Cache by font face, glyph id/cluster, size, DPI, and color mode.

## Current implementation boundary

CervTerm removed the temporary external-process emoji path from the core rendering decision and now uses a small renderer-neutral abstraction in `internal/fontglyph` (`Backend` / `RasterizedGlyph`). The current backend has monochrome OpenType rendering, initial bitmap color glyph paths for `sbix` and `CBDT/CBLC`, COLRv0/CPAL layer extraction/rasterization, a broader COLRv1 paint graph subset (`PaintGlyph`, `PaintSolid`, `PaintLinearGradient`, `PaintRadialGradient`, `PaintSweepGradient`, `PaintColrLayers`, `PaintColrGlyph`, `PaintComposite` Porter-Duff modes through `PLUS`, separable blend modes through `MULTIPLY`, and HSL blend modes, non-variable transform wrappers, and ItemVariationStore-backed `PaintVar*` deltas for alpha/translation/scale/rotate/skew/gradient-coordinate/color-line-stop fields), OpenType `SVG ` extraction plus minimal `rect`/`circle`/line-path/quadratic-path/cubic-path/solid-text+tspan/`linearGradient` rasterization with basic text-anchor/baseline/font-size handling, small redistributable synthetic `SVG `/COLR/CPAL table fixtures including PaintVarScale and PaintComposite coverage plus redistributable Go-font real TTF tests and a licensed Noto Color Emoji subset fixture, optional system color-font detection, a real known-emoji backend rasterization fixture, and a representative multi-glyph emoji rasterization fixture when such a system font exists. `RasterizeCluster` is now used by the GLFW renderer for combining marks, variation selectors, emoji modifiers, and ZWJ emoji sequences, and canonical combining clusters now try NFC composition before falling back to string drawing. A renderer-neutral `Shaper` interface is in place, with `SimpleShaper` as the default pure-Go sfnt glyph-ID/advance backend for simple clusters, and a Windows-only DirectWriteShaper entrypoint selected when `IDWriteFactory`/`IDWriteTextAnalyzer` creation succeeds; fallback font paths are retained and can be opened as `IDWriteFontFace` objects, and `GetGlyphs`/`GetGlyphPlacements` now map DirectWrite glyph IDs/advances/offsets into `ShapedGlyph` for source-backed complex clusters; tests cover Arabic, Indic, and Segoe UI Emoji ZWJ smoke cases. Shaped COLR clusters now render through the color path for single-glyph clusters such as Segoe UI Emoji ZWJ and multi-glyph clusters covered by the Noto Color Emoji subset fixture. Remaining real SVG text font selection/outline layout and broader real-world shaped color coverage remain future work. This preserves the important separation: font metrics and glyph rasterization are outside the GLFW/OpenGL atlas, and terminal-specific cell rendering stays inside CervTerm.

See [`docs/gogpu-color-glyph-extraction.md`](gogpu-color-glyph-extraction.md) for what belongs in a future GoGPU contribution versus what remains terminal-specific.

See also [`docs/shaping-options.md`](shaping-options.md) for the concrete shaping engine tradeoffs.
