package fontglyph

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"sync"

	"cervterm/internal/fontdesc"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type ShapedGlyph struct {
	GlyphID  uint16
	XOffset  float64
	YOffset  float64
	XAdvance float64
}

type Shaper interface {
	Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool)
}

type FeatureShaper interface {
	ShapeFeatures(cluster string, face loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool)
}

func shapeWithFeatures(shaper Shaper, cluster string, face loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool) {
	if featureShaper, ok := shaper.(FeatureShaper); ok {
		return featureShaper.ShapeFeatures(cluster, face, ppem, features)
	}
	return shaper.Shape(cluster, face, ppem)
}

var portableFeatureDiagnosticOnce sync.Once

// ConfigureBackendFeatures installs one immutable effective feature set into a
// context-local backend graph. Parsed font cache entries remain feature-neutral.
func ConfigureBackendFeatures(backend Backend, features fontdesc.FeatureSet) {
	var configureOpenType func(*OpenTypeBackend)
	configureOpenType = func(item *OpenTypeBackend) {
		if item == nil {
			return
		}
		item.features = features
		if features.RequestsFeatureCapability() {
			if _, portable := item.shaper.(SimpleShaper); portable {
				portableFeatureDiagnosticOnce.Do(func() {
					log.Printf("font feature capability: portable SimpleShaper preserves the fixed grid but does not apply OpenType substitutions")
				})
			}
		}
	}
	switch typed := backend.(type) {
	case *OpenTypeBackend:
		configureOpenType(typed)
	case *descriptorBackend:
		for _, item := range typed.backends {
			configureOpenType(item)
		}
	case *fallbackBackend:
		typed.features = features
		if typed.primary != nil {
			for _, item := range typed.primary.backends {
				configureOpenType(item)
			}
		}
		for _, item := range typed.loaded {
			configureOpenType(item)
		}
	}
}

type featureCapabilityReporter interface {
	FeatureCapability() string
}

// BackendFeatureCapability reports whether the active platform shaper applies
// configured OpenType features without exposing font paths or glyph data.
func BackendFeatureCapability(backend Backend) string {
	var shaper Shaper
	switch typed := backend.(type) {
	case *OpenTypeBackend:
		shaper = typed.shaper
	case *descriptorBackend:
		if typed.backends[fontdesc.RequestedFaceStyleNormal] != nil {
			shaper = typed.backends[fontdesc.RequestedFaceStyleNormal].shaper
		}
	case *fallbackBackend:
		if typed.primary != nil && typed.primary.backends[fontdesc.RequestedFaceStyleNormal] != nil {
			shaper = typed.primary.backends[fontdesc.RequestedFaceStyleNormal].shaper
		}
	}
	if reporter, ok := shaper.(featureCapabilityReporter); ok {
		return reporter.FeatureCapability()
	}
	return "unsupported"
}

func (b *OpenTypeBackend) SetShaper(shaper Shaper) {
	b.shaper = shaper
}

func centerShapedGlyphsInCells(shaped []ShapedGlyph, cellPixels int) []ShapedGlyph {
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
	centered := append([]ShapedGlyph(nil), shaped...)
	// rasterizeShapedCluster starts at x=1; offset that origin so the shaped
	// advance box is centered exactly like the fixed-grid per-rune path.
	centered[0].XOffset += (float64(cellPixels)-advance)/2 - 1
	return centered
}

func (b *OpenTypeBackend) rasterizeShapedCluster(lf loadedFace, shaped []ShapedGlyph, cellSpan int) (RasterizedGlyph, bool) {
	if lf.sfnt == nil || len(shaped) == 0 {
		return RasterizedGlyph{}, false
	}
	if glyph, ok := b.rasterizeShapedBitmapColorCluster(lf, shaped, cellSpan); ok {
		return glyph, true
	}
	if glyph, ok := b.rasterizeShapedColorCluster(lf, shaped, cellSpan); ok {
		return glyph, true
	}
	img := image.NewRGBA(image.Rect(0, 0, b.cellW*cellSpan, b.cellH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	var buf sfnt.Buffer
	pen := 0.0
	maxAdvance := 0.0
	ppem := fixed.I(int(b.ppem))
	for _, glyph := range shaped {
		if glyph.GlyphID == 0 {
			continue
		}
		segments, err := lf.sfnt.LoadGlyph(&buf, sfnt.GlyphIndex(glyph.GlyphID), ppem, nil)
		if err != nil {
			return RasterizedGlyph{}, false
		}
		drawSegments(img, segments, color.RGBA{255, 255, 255, 255}, 1+int(math.Round(pen+glyph.XOffset)), b.baseline+int(math.Round(glyph.YOffset)), identityCOLRTransform())
		pen += glyph.XAdvance
		if pen > maxAdvance {
			maxAdvance = pen
		}
	}
	if !hasVisibleRGBA(img) {
		return RasterizedGlyph{}, false
	}
	if maxAdvance <= 0 {
		maxAdvance = float64(b.cellW * cellSpan)
	}
	return RasterizedGlyph{
		Image:    img,
		Width:    max(1, int(math.Ceil(maxAdvance))),
		Height:   b.cellH,
		BearingX: 1,
		BearingY: b.baseline,
		AdvanceX: maxAdvance,
		CellSpan: cellSpan,
		HasColor: false,
	}, true
}

func (b *OpenTypeBackend) rasterizeShapedBitmapColorCluster(lf loadedFace, shaped []ShapedGlyph, cellSpan int) (RasterizedGlyph, bool) {
	if len(shaped) == 0 {
		return RasterizedGlyph{}, false
	}
	bitmaps := make([]bitmapGlyph, 0, len(shaped))
	for _, glyph := range shaped {
		if glyph.GlyphID == 0 {
			return RasterizedGlyph{}, false
		}
		bitmap, ok := bitmapColorGlyph(lf, glyph.GlyphID, b.ppem)
		if !ok {
			return RasterizedGlyph{}, false
		}
		bitmaps = append(bitmaps, bitmap)
	}
	cellSpan = max(1, cellSpan)
	canvasW := b.cellW * cellSpan
	canvasH := b.cellH
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	advance := 0.0
	if len(bitmaps) == 1 {
		if !drawBitmapGlyphFit(img, bitmaps[0], image.Rect(0, 0, canvasW, canvasH)) {
			return RasterizedGlyph{}, false
		}
		advance = shaped[0].XAdvance
		if advance <= 0 {
			advance = float64(canvasW)
		}
	} else {
		slotW := max(1, canvasW/len(bitmaps))
		for i, bitmap := range bitmaps {
			slot := image.Rect(i*slotW, 0, canvasW, canvasH)
			if i < len(bitmaps)-1 {
				slot.Max.X = (i + 1) * slotW
			}
			if !drawBitmapGlyphFit(img, bitmap, slot) {
				return RasterizedGlyph{}, false
			}
			advance += shaped[i].XAdvance
		}
		if advance <= 0 {
			advance = float64(canvasW)
		}
	}

	if !hasVisibleRGBA(img) {
		return RasterizedGlyph{}, false
	}
	return RasterizedGlyph{
		Image:    img,
		Width:    canvasW,
		Height:   canvasH,
		BearingX: 0,
		BearingY: canvasH,
		AdvanceX: advance,
		CellSpan: cellSpan,
		HasColor: true,
	}, true
}

func drawBitmapGlyphFit(dst *image.RGBA, bitmap bitmapGlyph, slot image.Rectangle) bool {
	srcBounds := bitmap.Image.Bounds()
	if srcBounds.Dx() <= 0 || srcBounds.Dy() <= 0 || slot.Dx() <= 0 || slot.Dy() <= 0 {
		return false
	}
	scale := math.Min(float64(slot.Dx())/float64(srcBounds.Dx()), float64(slot.Dy())/float64(srcBounds.Dy()))
	if scale <= 0 {
		return false
	}
	dstW := max(1, int(math.Round(float64(srcBounds.Dx())*scale)))
	dstH := max(1, int(math.Round(float64(srcBounds.Dy())*scale)))
	dstX := slot.Min.X + (slot.Dx()-dstW)/2
	dstY := slot.Min.Y + (slot.Dy()-dstH)/2
	target := image.Rect(dstX, dstY, dstX+dstW, dstY+dstH)
	xdraw.CatmullRom.Scale(dst, target, bitmap.Image, srcBounds, xdraw.Over, nil)
	return true
}

func hasVisibleRGBA(img *image.RGBA) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.RGBAAt(x, y).A != 0 {
				return true
			}
		}
	}
	return false
}
