package fontglyph

import (
	"unicode"

	"cervterm/internal/unicodeprops"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type SimpleShaper struct{}

func (SimpleShaper) Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	if cluster == "" || face.sfnt == nil {
		return nil, false
	}
	if r, ok := normalizeClusterToSingleRune(cluster); ok {
		return shapeOneRune(face.sfnt, r, ppem)
	}
	if !isSimpleShapeableCluster(cluster) {
		return nil, false
	}
	var out []ShapedGlyph
	for _, r := range cluster {
		shaped, ok := shapeOneRune(face.sfnt, r, ppem)
		if !ok {
			return nil, false
		}
		out = append(out, shaped...)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func shapeOneRune(sfntFont *sfnt.Font, r rune, ppem uint16) ([]ShapedGlyph, bool) {
	var buf sfnt.Buffer
	glyphID, err := sfntFont.GlyphIndex(&buf, r)
	if err != nil || glyphID == 0 {
		return nil, false
	}
	advance, err := sfntFont.GlyphAdvance(&buf, glyphID, fixed.I(int(ppem)), font.HintingFull)
	if err != nil {
		return nil, false
	}
	return []ShapedGlyph{{GlyphID: uint16(glyphID), XAdvance: float64(advance) / 64.0}}, true
}

func isSimpleShapeableCluster(cluster string) bool {
	for _, r := range cluster {
		if isComplexShapingRune(r) {
			return false
		}
	}
	return true
}

func isComplexShapingRune(r rune) bool {
	if unicodeprops.IsEmojiControl(r) {
		return true
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Mc, r) {
		return true
	}
	if unicode.In(r, unicode.Arabic, unicode.Devanagari, unicode.Bengali, unicode.Gurmukhi, unicode.Gujarati, unicode.Oriya, unicode.Tamil, unicode.Telugu, unicode.Kannada, unicode.Malayalam, unicode.Thai, unicode.Lao, unicode.Tibetan, unicode.Khmer, unicode.Myanmar) {
		return true
	}
	return false
}
