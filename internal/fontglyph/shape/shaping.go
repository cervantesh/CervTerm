package shape

import (
	"unicode"
	"unicode/utf8"

	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph/internal/face"
	"cervterm/internal/unicodeprops"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/norm"
)

// FeatureCapability reports the portable shaper's fixed capability string.
func (Simple) FeatureCapability() string { return "portable-unsupported" }

// ShapeFeatures preserves output while accepting the effective feature set.
func (s Simple) ShapeFeatures(cluster string, ref face.Ref, ppem uint16, _ fontdesc.FeatureSet) ([]Glyph, bool) {
	return s.Shape(cluster, ref, ppem)
}

// SingleSimpleRune recognizes the allocation-free one-rune fast path while
// preserving complex-script rejection.
func SingleSimpleRune(cluster string) (rune, bool) {
	value, size := utf8.DecodeRuneInString(cluster)
	if size == 0 || size != len(cluster) || value == utf8.RuneError && size == 1 || IsComplexRune(value) {
		return 0, false
	}
	return value, true
}

// Shape maps simple clusters without script substitution.
func (Simple) Shape(cluster string, ref face.Ref, ppem uint16) ([]Glyph, bool) {
	if cluster == "" || !ref.Valid() {
		return nil, false
	}
	if value, ok := SingleSimpleRune(cluster); ok {
		return ShapeOneRune(ref, value, ppem)
	}
	if value, ok := NormalizeSingleRune(cluster); ok {
		return ShapeOneRune(ref, value, ppem)
	}
	if !IsSimpleCluster(cluster) {
		return nil, false
	}
	var out []Glyph
	for _, value := range cluster {
		shaped, ok := ShapeOneRune(ref, value, ppem)
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

// OneRune maps one scalar without allocating an output slice.
func OneRune(ref face.Ref, value rune, ppem uint16) (Glyph, bool) {
	if !ref.Valid() {
		return Glyph{}, false
	}
	var buffer sfnt.Buffer
	glyphID, err := ref.GlyphIndex(&buffer, value)
	if err != nil || glyphID == 0 {
		return Glyph{}, false
	}
	advance, err := ref.GlyphAdvance(&buffer, glyphID, fixed.I(int(ppem)), font.HintingFull)
	if err != nil {
		return Glyph{}, false
	}
	return Glyph{GlyphID: uint16(glyphID), XAdvance: float64(advance) / 64.0}, true
}

// ShapeOneRune preserves the slice-returning subsystem contract.
func ShapeOneRune(ref face.Ref, value rune, ppem uint16) ([]Glyph, bool) {
	glyph, ok := OneRune(ref, value, ppem)
	if !ok {
		return nil, false
	}
	return []Glyph{glyph}, true
}

// NormalizeSingleRune applies NFC and reports only changed single-rune clusters.
func NormalizeSingleRune(cluster string) (rune, bool) {
	normalized := norm.NFC.String(cluster)
	var out rune
	count := 0
	for _, value := range normalized {
		out = value
		count++
		if count > 1 {
			return 0, false
		}
	}
	if count != 1 {
		return 0, false
	}
	return out, normalized != cluster
}

// IsSimpleCluster reports whether portable per-rune shaping is safe.
func IsSimpleCluster(cluster string) bool {
	for _, value := range cluster {
		if IsComplexRune(value) {
			return false
		}
	}
	return true
}

// IsComplexRune identifies controls, combining marks, and scripts requiring shaping.
func IsComplexRune(value rune) bool {
	if unicodeprops.IsEmojiControl(value) {
		return true
	}
	if unicode.Is(unicode.Mn, value) || unicode.Is(unicode.Me, value) || unicode.Is(unicode.Mc, value) {
		return true
	}
	return unicode.In(value, unicode.Arabic, unicode.Devanagari, unicode.Bengali, unicode.Gurmukhi, unicode.Gujarati, unicode.Oriya, unicode.Tamil, unicode.Telugu, unicode.Kannada, unicode.Malayalam, unicode.Thai, unicode.Lao, unicode.Tibetan, unicode.Khmer, unicode.Myanmar)
}

// WithFeatures dispatches to an optional feature-aware implementation.
func WithFeatures(shaper Shaper, cluster string, ref face.Ref, ppem uint16, features fontdesc.FeatureSet) ([]Glyph, bool) {
	if shaper == nil {
		return nil, false
	}
	if featureShaper, ok := shaper.(FeatureShaper); ok {
		return featureShaper.ShapeFeatures(cluster, ref, ppem, features)
	}
	return shaper.Shape(cluster, ref, ppem)
}

// Default returns the injected platform shaper or the portable concrete default.
func Default(factory PlatformFactory) Shaper {
	portable := Simple{}
	if factory != nil {
		if platform := factory(portable); platform != nil {
			return platform
		}
	}
	return portable
}

// CenterInCells returns detached output and centers the shaped advance box.
func CenterInCells(shaped []Glyph, cellPixels int) []Glyph {
	if len(shaped) == 0 || cellPixels <= 0 {
		return shaped
	}
	advance := 0.0
	for _, glyph := range shaped {
		advance += glyph.XAdvance
	}
	if advance <= 0 {
		return shaped
	}
	centered := append([]Glyph(nil), shaped...)
	centered[0].XOffset += (float64(cellPixels)-advance)/2 - 1
	return centered
}

// RunSubstituted distinguishes GSUB output from advance-only GPOS changes.
func RunSubstituted(shaper Shaper, ref face.Ref, ppem uint16, run string, shaped []Glyph, features fontdesc.FeatureSet) bool {
	runeCount := 0
	perRune := make([]Glyph, 0, len(shaped))
	perRuneOK := true
	for _, value := range run {
		runeCount++
		if !perRuneOK {
			continue
		}
		glyphs, ok := WithFeatures(shaper, string(value), ref, ppem, features)
		if !ok {
			perRuneOK = false
			continue
		}
		perRune = append(perRune, glyphs...)
	}
	if len(shaped) < runeCount {
		return true
	}
	if !perRuneOK {
		return false
	}
	if len(shaped) != len(perRune) {
		return true
	}
	for index := range shaped {
		if shaped[index].GlyphID != perRune[index].GlyphID {
			return true
		}
	}
	return false
}
