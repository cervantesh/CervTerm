package raster

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

type Renderer struct{ Metrics }

// RasterizeCOLRGlyph renders one COLR glyph with deterministic paint order.
func (b *Renderer) RasterizeCOLRGlyph(lf Face, r rune, cellSpan int, bounds fixed.Rectangle26_6, advance fixed.Int26_6) (Glyph, bool) {
	if lf.Parsed == nil || lf.Color == nil || !lf.Color.HasCOLR() {
		return Glyph{}, false
	}
	var buf sfnt.Buffer
	glyphID, err := lf.Parsed.GlyphIndex(&buf, r)
	if err != nil || glyphID == 0 {
		return Glyph{}, false
	}
	colrGlyph, err := colorGlyphForFace(lf, uint16(glyphID))
	if err != nil {
		return Glyph{}, false
	}
	canvasW := b.CellWidth * cellSpan
	canvasH := b.CellHeight
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	if !b.renderCOLRLayers(img, lf, &buf, colrGlyph.Layers) {
		return Glyph{}, false
	}
	if isSegoeEmojiSource(lf.SourcePath) {
		img = fitColorGlyphToCanvas(img, 0)
	}
	return Glyph{
		Image:    img,
		Width:    (bounds.Max.X - bounds.Min.X).Ceil(),
		Height:   (bounds.Max.Y - bounds.Min.Y).Ceil(),
		BearingX: bounds.Min.X.Ceil(),
		BearingY: -bounds.Min.Y.Ceil(),
		AdvanceX: float64(advance) / 64.0,
		CellSpan: cellSpan,
		HasColor: true,
	}, true
}

// RasterizeShapedColorCluster renders one positioned COLR glyph run.
func (b *Renderer) RasterizeShapedColorCluster(lf Face, shaped []ShapeGlyph, cellSpan int) (Glyph, bool) {
	if lf.Parsed == nil || lf.Color == nil || !lf.Color.HasCOLR() || len(shaped) == 0 {
		return Glyph{}, false
	}
	canvasW := b.CellWidth * max(1, cellSpan)
	canvasH := b.CellHeight
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	var buf sfnt.Buffer
	pen := 0.0
	totalAdvance := 0.0
	for _, glyph := range shaped {
		if glyph.GlyphID == 0 {
			return Glyph{}, false
		}
		colrGlyph, err := colorGlyphForFace(lf, glyph.GlyphID)
		if err != nil {
			return Glyph{}, false
		}
		offsetX := 1 + int(math.Round(pen+glyph.XOffset))
		baseline := b.Baseline - int(math.Round(glyph.YOffset))
		if !b.renderCOLRLayersAt(img, lf, &buf, colrGlyph.Layers, offsetX, baseline) {
			return Glyph{}, false
		}
		advance := glyph.XAdvance
		if advance <= 0 {
			advance = float64(b.CellWidth)
		}
		pen += advance
		totalAdvance = pen
	}
	if !hasVisibleRGBA(img) {
		return Glyph{}, false
	}
	if isSegoeEmojiSource(lf.SourcePath) {
		img = fitColorGlyphToCanvas(img, 0)
	}
	if totalAdvance <= 0 {
		totalAdvance = float64(canvasW)
	}
	return Glyph{
		Image:    img,
		Width:    max(1, int(math.Ceil(totalAdvance))),
		Height:   b.CellHeight,
		BearingX: 1,
		BearingY: b.Baseline,
		AdvanceX: totalAdvance,
		CellSpan: max(1, cellSpan),
		HasColor: true,
	}, true
}

// RasterizeShapedMonochrome renders one positioned outline run.
func (b *Renderer) RasterizeShapedMonochrome(lf Face, shaped []ShapeGlyph, cellSpan int) (Glyph, bool) {
	if lf.Parsed == nil || len(shaped) == 0 {
		return Glyph{}, false
	}
	img := image.NewRGBA(image.Rect(0, 0, b.CellWidth*cellSpan, b.CellHeight))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	var buffer sfnt.Buffer
	pen := 0.0
	maxAdvance := 0.0
	ppem := fixed.I(int(b.PPEM))
	for _, glyph := range shaped {
		if glyph.GlyphID == 0 {
			continue
		}
		segments, err := lf.Parsed.LoadGlyph(&buffer, sfnt.GlyphIndex(glyph.GlyphID), ppem, nil)
		if err != nil {
			return Glyph{}, false
		}
		drawSegments(img, segments, color.RGBA{255, 255, 255, 255}, 1+int(math.Round(pen+glyph.XOffset)), b.Baseline+int(math.Round(glyph.YOffset)), identityCOLRTransform())
		pen += glyph.XAdvance
		if pen > maxAdvance {
			maxAdvance = pen
		}
	}
	if !hasVisibleRGBA(img) {
		return Glyph{}, false
	}
	if maxAdvance <= 0 {
		maxAdvance = float64(b.CellWidth * cellSpan)
	}
	return Glyph{Image: img, Width: max(1, int(math.Ceil(maxAdvance))), Height: b.CellHeight, BearingX: 1, BearingY: b.Baseline, AdvanceX: maxAdvance, CellSpan: cellSpan}, true
}

func colorGlyphForFace(lf Face, glyphID uint16) (COLRGlyph, error) {
	if lf.Color == nil {
		return COLRGlyph{}, ErrNoCOLRTable
	}
	return lf.Color.COLRGlyph(glyphID, isSegoeEmojiSource(lf.SourcePath))
}
func fitColorGlyphToCanvas(img *image.RGBA, padding int) *image.RGBA {
	if img == nil {
		return nil
	}
	visible, ok := visibleRGBABounds(img)
	if !ok {
		return img
	}
	canvas := img.Bounds()
	maxW := canvas.Dx() - 2*padding
	maxH := canvas.Dy() - 2*padding
	if maxW <= 0 || maxH <= 0 || visible.Dx() <= 0 || visible.Dy() <= 0 {
		return img
	}
	scale := math.Min(float64(maxW)/float64(visible.Dx()), float64(maxH)/float64(visible.Dy()))
	if scale <= 0 {
		return img
	}
	dstW := max(1, int(math.Round(float64(visible.Dx())*scale)))
	dstH := max(1, int(math.Round(float64(visible.Dy())*scale)))
	dstX := canvas.Min.X + (canvas.Dx()-dstW)/2
	dstY := canvas.Min.Y + (canvas.Dy()-dstH)/2
	out := image.NewRGBA(canvas)
	draw.Draw(out, out.Bounds(), image.Transparent, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(out, image.Rect(dstX, dstY, dstX+dstW, dstY+dstH), img, visible, xdraw.Over, nil)
	return out
}

func visibleRGBABounds(img *image.RGBA) (image.Rectangle, bool) {
	if img == nil {
		return image.Rectangle{}, false
	}
	bounds := img.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X, bounds.Min.Y
	found := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.RGBAAt(x, y).A == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x+1 > maxX {
				maxX = x + 1
			}
			if y+1 > maxY {
				maxY = y + 1
			}
			found = true
		}
	}
	if !found {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX, maxY), true
}

func (b *Renderer) renderCOLRLayers(dst *image.RGBA, lf Face, buf *sfnt.Buffer, layers []COLRLayer) bool {
	return b.renderCOLRLayersAt(dst, lf, buf, layers, 1, b.Baseline)
}

func (b *Renderer) renderCOLRLayersAt(dst *image.RGBA, lf Face, buf *sfnt.Buffer, layers []COLRLayer, offsetX int, baseline int) bool {
	ppem := fixed.I(int(b.PPEM))
	for _, layer := range layers {
		if layer.Fill == COLRFillComposite {
			if !b.renderCompositeLayerAt(dst, lf, buf, layer, offsetX, baseline) {
				return false
			}
			continue
		}
		segments, err := lf.Parsed.LoadGlyph(buf, sfnt.GlyphIndex(layer.GlyphID), ppem, nil)
		if err != nil {
			return false
		}
		if layer.Fill == COLRFillLinearGradient || layer.Fill == COLRFillRadialGradient || layer.Fill == COLRFillSweepGradient {
			drawGradientSegments(dst, segments, layer, offsetX, baseline)
			continue
		}
		fill := layer.Color
		if layer.Foreground {
			fill = color.RGBA{255, 255, 255, 255}
		}
		drawSegments(dst, segments, fill, offsetX, baseline, layer.Transform)
	}
	return true
}

func (b *Renderer) renderCompositeLayer(dst *image.RGBA, lf Face, buf *sfnt.Buffer, layer COLRLayer) bool {
	return b.renderCompositeLayerAt(dst, lf, buf, layer, 1, b.Baseline)
}

func (b *Renderer) renderCompositeLayerAt(dst *image.RGBA, lf Face, buf *sfnt.Buffer, layer COLRLayer, offsetX int, baseline int) bool {
	source := image.NewRGBA(dst.Bounds())
	backdrop := image.NewRGBA(dst.Bounds())
	if !b.renderCOLRLayersAt(source, lf, buf, layer.Source, offsetX, baseline) || !b.renderCOLRLayersAt(backdrop, lf, buf, layer.Backdrop, offsetX, baseline) {
		return false
	}
	result := image.NewRGBA(dst.Bounds())
	bounds := dst.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			result.SetRGBA(x, y, compositeCOLRPixel(source.RGBAAt(x, y), backdrop.RGBAAt(x, y), layer.CompositeMode))
		}
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			overRGBA(dst, x, y, result.RGBAAt(x, y))
		}
	}
	return true
}

func drawSegments(dst draw.Image, segments sfnt.Segments, fill color.RGBA, offsetX int, baseline int, transform COLRTransform) {
	r := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
	for _, segment := range segments {
		switch segment.Op {
		case sfnt.SegmentOpMoveTo:
			x, y := pointXY(segment.Args[0], offsetX, baseline, transform)
			r.MoveTo(x, y)
		case sfnt.SegmentOpLineTo:
			x, y := pointXY(segment.Args[0], offsetX, baseline, transform)
			r.LineTo(x, y)
		case sfnt.SegmentOpQuadTo:
			bx, by := pointXY(segment.Args[0], offsetX, baseline, transform)
			cx, cy := pointXY(segment.Args[1], offsetX, baseline, transform)
			r.QuadTo(bx, by, cx, cy)
		case sfnt.SegmentOpCubeTo:
			bx, by := pointXY(segment.Args[0], offsetX, baseline, transform)
			cx, cy := pointXY(segment.Args[1], offsetX, baseline, transform)
			dx, dy := pointXY(segment.Args[2], offsetX, baseline, transform)
			r.CubeTo(bx, by, cx, cy, dx, dy)
		}
	}
	r.Draw(dst, dst.Bounds(), image.NewUniform(fill), image.Point{})
}

func drawGradientSegments(dst *image.RGBA, segments sfnt.Segments, layer COLRLayer, offsetX int, baseline int) {
	mask := image.NewRGBA(dst.Bounds())
	drawSegments(mask, segments, color.RGBA{255, 255, 255, 255}, offsetX, baseline, layer.Transform)
	linear := layer.LinearGradient
	radial := layer.RadialGradient
	sweep := layer.SweepGradient
	if layer.Fill == COLRFillLinearGradient {
		linear.X0, linear.Y0 = layer.Transform.Apply(linear.X0, linear.Y0)
		linear.X1, linear.Y1 = layer.Transform.Apply(linear.X1, linear.Y1)
		linear.X2, linear.Y2 = layer.Transform.Apply(linear.X2, linear.Y2)
		linear.X0 += float64(offsetX)
		linear.X1 += float64(offsetX)
		linear.X2 += float64(offsetX)
		linear.Y0 += float64(baseline)
		linear.Y1 += float64(baseline)
		linear.Y2 += float64(baseline)
	} else if layer.Fill == COLRFillRadialGradient {
		radial.X0, radial.Y0 = layer.Transform.Apply(radial.X0, radial.Y0)
		radial.X1, radial.Y1 = layer.Transform.Apply(radial.X1, radial.Y1)
		radial.X0 += float64(offsetX)
		radial.X1 += float64(offsetX)
		radial.Y0 += float64(baseline)
		radial.Y1 += float64(baseline)
	} else {
		sweep.CenterX, sweep.CenterY = layer.Transform.Apply(sweep.CenterX, sweep.CenterY)
		sweep.CenterX += float64(offsetX)
		sweep.CenterY += float64(baseline)
	}
	bounds := dst.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			maskAlpha := mask.RGBAAt(x, y).A
			if maskAlpha == 0 {
				continue
			}
			var src color.RGBA
			switch layer.Fill {
			case COLRFillLinearGradient:
				src = linear.colorAt(float64(x)+0.5, float64(y)+0.5)
			case COLRFillRadialGradient:
				src = radial.colorAt(float64(x)+0.5, float64(y)+0.5)
			case COLRFillSweepGradient:
				src = sweep.colorAt(float64(x)+0.5, float64(y)+0.5)
			}
			src.A = uint8((uint16(src.A)*uint16(maskAlpha) + 127) / 255)
			overRGBA(dst, x, y, src)
		}
	}
}

func pointXY(p fixed.Point26_6, offset int, baseline int, transform COLRTransform) (float32, float32) {
	x, y := transform.Apply(float64(p.X)/64, float64(p.Y)/64)
	return float32(offset) + float32(x), float32(baseline) + float32(y)
}

func isSegoeEmojiSource(source string) bool {
	return strings.EqualFold(filepath.Base(source), "seguiemj.ttf")
}

func hasVisibleRGBA(img *image.RGBA) bool {
	if img == nil {
		return false
	}
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if img.RGBAAt(x, y).A != 0 {
				return true
			}
		}
	}
	return false
}

// HasVisibleRGBA reports whether an RGBA image contains non-zero alpha.
func HasVisibleRGBA(img *image.RGBA) bool { return hasVisibleRGBA(img) }

// VisibleRGBABounds returns the exact non-transparent pixel bounds.
func VisibleRGBABounds(img *image.RGBA) (image.Rectangle, bool) {
	return visibleRGBABounds(img)
}
