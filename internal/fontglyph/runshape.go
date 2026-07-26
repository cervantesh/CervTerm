package fontglyph

import (
	"cervterm/internal/fontdesc"
	shapepkg "cervterm/internal/fontglyph/shape"
)

// SupportsLigatures reports whether the active shaper can produce GSUB
// substitutions across multiple cells. The pure-Go SimpleShaper only maps one
// glyph per input rune, so ligatures require an advanced shaper (DirectWrite).
// Callers gate the feature on this once so no per-frame probing happens.
func (b *OpenTypeBackend) SupportsLigatures() bool {
	if b == nil || b.shaper == nil {
		return false
	}
	return !isPortableShaper(b.shaper)
}

// RasterizeRun shapes a multi-cell run of programming-symbol characters and, if
// the configured font substitutes a ligature glyph across them, rasterizes the
// shaped glyphs into a single cellSpan-wide bitmap. The grid always wins: the
// returned glyph is forced to occupy exactly cellSpan cells regardless of the
// advances the shaper returns.
//
// The bool reports whether a ligature substitution actually occurred. A run
// whose whole-run shaping is one glyph per input rune with the same glyph IDs
// as shaping each rune alone is NOT a ligature — some fonts kern symbol pairs
// via GPOS only, and advance-only changes must not break the monospace grid.
// In that case, and on any shaper failure, the bool is false and the caller
// renders the run per-cell (never a blank span).
func (b *OpenTypeBackend) RasterizeRun(run string, cellSpan int) (RasterizedGlyph, bool) {
	if b == nil || b.shaper == nil || run == "" {
		return RasterizedGlyph{}, false
	}
	lf, ok := firstTextFaceForCluster(run, b.faceForRune)
	if !ok || lf.sfnt == nil {
		return RasterizedGlyph{}, false
	}
	shaped, ok := shapeWithFeatures(b.shaper, run, lf, b.ppem, b.features)
	if !ok || len(shaped) == 0 {
		return RasterizedGlyph{}, false
	}
	if !runSubstituted(b.shaper, lf, b.ppem, run, shaped, b.features) {
		return RasterizedGlyph{}, false
	}
	cellSpan = max(1, cellSpan)
	glyph, ok := b.rasterizeShapedCluster(lf, shaped, cellSpan)
	if !ok {
		return RasterizedGlyph{}, false
	}
	// Grid wins: the run occupies exactly cellSpan cells however wide the font
	// drew the ligature, so the atlas draws it across the run's cells 1:1.
	glyph.CellSpan = cellSpan
	glyph.AdvanceX = float64(b.cellW * cellSpan)
	return glyph, true
}

// runSubstituted keeps the historical root helper while shape owns the
// GSUB-vs-GPOS decision and all per-rune comparison behavior.
func runSubstituted(shaper Shaper, source loadedFace, ppem uint16, run string, shaped []ShapedGlyph, features fontdesc.FeatureSet) bool {
	return shapepkg.RunSubstituted(rootToShapeShaper{root: shaper}, shapingFaceRef(source), ppem, run, shapedGlyphsToShape(shaped), features)
}
