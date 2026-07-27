package face

import (
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Ref is a detached, immutable shaping view of a parsed face. It deliberately
// excludes raster, color, cache-lease, and native-handle ownership.
type Ref struct {
	font   *sfnt.Font
	source string
	index  int
}

// NewRef projects the minimum parsed-face state needed by a shaper.
func NewRef(parsed *sfnt.Font, canonicalSource string, collectionIndex int) Ref {
	return Ref{font: parsed, source: canonicalSource, index: collectionIndex}
}

// Valid reports whether the reference contains a parsed SFNT face.
func (r Ref) Valid() bool { return r.font != nil }

// Parsed returns the immutable SFNT parser shared by shaping adapters. It does
// not transfer ownership or expose source bytes, leases, or native resources.
func (r Ref) Parsed() *sfnt.Font { return r.font }

// Source returns the stable source and collection index needed by an injected
// platform shaper. The bool is false when no source was projected.
func (r Ref) Source() (string, int, bool) {
	return r.source, r.index, r.source != ""
}

// GlyphIndex resolves one rune without exposing the parsed face pointer.
func (r Ref) GlyphIndex(buffer *sfnt.Buffer, value rune) (sfnt.GlyphIndex, error) {
	return r.font.GlyphIndex(buffer, value)
}

// GlyphAdvance reads one glyph advance without exposing raster ownership.
func (r Ref) GlyphAdvance(buffer *sfnt.Buffer, glyph sfnt.GlyphIndex, ppem fixed.Int26_6, hinting font.Hinting) (fixed.Int26_6, error) {
	return r.font.GlyphAdvance(buffer, glyph, ppem, hinting)
}
