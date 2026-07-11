package fontglyph

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

func (b *OpenTypeBackend) rasterizeCOLRGlyph(lf loadedFace, r rune, cellSpan int, bounds fixed.Rectangle26_6, advance fixed.Int26_6) (RasterizedGlyph, bool) {
	if lf.sfnt == nil || lf.colr == nil {
		return RasterizedGlyph{}, false
	}
	var buf sfnt.Buffer
	glyphID, err := lf.sfnt.GlyphIndex(&buf, r)
	if err != nil || glyphID == 0 {
		return RasterizedGlyph{}, false
	}
	colrGlyph, err := colorGlyphForFace(lf, uint16(glyphID))
	if err != nil {
		return RasterizedGlyph{}, false
	}
	canvasW := b.cellW * cellSpan
	canvasH := b.cellH
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	if !b.renderCOLRLayers(img, lf, &buf, colrGlyph.Layers) {
		return RasterizedGlyph{}, false
	}
	if isSegoeEmojiFace(lf) {
		img = fitColorGlyphToCanvas(img, 0)
	}
	return RasterizedGlyph{
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

func (b *OpenTypeBackend) rasterizeShapedColorCluster(lf loadedFace, shaped []ShapedGlyph, cellSpan int) (RasterizedGlyph, bool) {
	if lf.sfnt == nil || lf.colr == nil || len(shaped) == 0 {
		return RasterizedGlyph{}, false
	}
	canvasW := b.cellW * max(1, cellSpan)
	canvasH := b.cellH
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	var buf sfnt.Buffer
	pen := 0.0
	totalAdvance := 0.0
	for _, glyph := range shaped {
		if glyph.GlyphID == 0 {
			return RasterizedGlyph{}, false
		}
		colrGlyph, err := colorGlyphForFace(lf, glyph.GlyphID)
		if err != nil {
			return RasterizedGlyph{}, false
		}
		offsetX := 1 + int(math.Round(pen+glyph.XOffset))
		baseline := b.baseline - int(math.Round(glyph.YOffset))
		if !b.renderCOLRLayersAt(img, lf, &buf, colrGlyph.Layers, offsetX, baseline) {
			return RasterizedGlyph{}, false
		}
		advance := glyph.XAdvance
		if advance <= 0 {
			advance = float64(b.cellW)
		}
		pen += advance
		totalAdvance = pen
	}
	if !hasVisibleRGBA(img) {
		return RasterizedGlyph{}, false
	}
	if isSegoeEmojiFace(lf) {
		img = fitColorGlyphToCanvas(img, 0)
	}
	if totalAdvance <= 0 {
		totalAdvance = float64(canvasW)
	}
	return RasterizedGlyph{
		Image:    img,
		Width:    max(1, int(math.Ceil(totalAdvance))),
		Height:   b.cellH,
		BearingX: 1,
		BearingY: b.baseline,
		AdvanceX: totalAdvance,
		CellSpan: max(1, cellSpan),
		HasColor: true,
	}, true
}

func colorGlyphForFace(lf loadedFace, glyphID uint16) (COLRGlyph, error) {
	if lf.colr == nil {
		return COLRGlyph{}, ErrNoCOLRTable
	}
	if isSegoeEmojiFace(lf) {
		if glyph, err := lf.colr.glyphV0(glyphID, lf.colr.palettes[0]); err == nil && len(glyph.Layers) > 0 {
			return glyph, nil
		}
	}
	return lf.colr.glyph(glyphID, 0)
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

func (b *OpenTypeBackend) renderCOLRLayers(dst *image.RGBA, lf loadedFace, buf *sfnt.Buffer, layers []COLRLayer) bool {
	return b.renderCOLRLayersAt(dst, lf, buf, layers, 1, b.baseline)
}

func (b *OpenTypeBackend) renderCOLRLayersAt(dst *image.RGBA, lf loadedFace, buf *sfnt.Buffer, layers []COLRLayer, offsetX int, baseline int) bool {
	ppem := fixed.I(int(b.ppem))
	for _, layer := range layers {
		if layer.Fill == COLRFillComposite {
			if !b.renderCompositeLayerAt(dst, lf, buf, layer, offsetX, baseline) {
				return false
			}
			continue
		}
		segments, err := lf.sfnt.LoadGlyph(buf, sfnt.GlyphIndex(layer.GlyphID), ppem, nil)
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

func (b *OpenTypeBackend) renderCompositeLayer(dst *image.RGBA, lf loadedFace, buf *sfnt.Buffer, layer COLRLayer) bool {
	return b.renderCompositeLayerAt(dst, lf, buf, layer, 1, b.baseline)
}

func (b *OpenTypeBackend) renderCompositeLayerAt(dst *image.RGBA, lf loadedFace, buf *sfnt.Buffer, layer COLRLayer, offsetX int, baseline int) bool {
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

func overRGBA(dst *image.RGBA, x int, y int, src color.RGBA) {
	if src.A == 0 {
		return
	}
	d := dst.RGBAAt(x, y)
	sa := uint32(src.A)
	inv := 255 - sa
	outA := sa + uint32(d.A)*inv/255
	if outA == 0 {
		dst.SetRGBA(x, y, color.RGBA{})
		return
	}
	dst.SetRGBA(x, y, color.RGBA{
		R: uint8((uint32(src.R)*sa + uint32(d.R)*uint32(d.A)*inv/255) / outA),
		G: uint8((uint32(src.G)*sa + uint32(d.G)*uint32(d.A)*inv/255) / outA),
		B: uint8((uint32(src.B)*sa + uint32(d.B)*uint32(d.A)*inv/255) / outA),
		A: uint8(outA),
	})
}

func compositeCOLRPixel(src, dst color.RGBA, mode int) color.RGBA {
	s := premulRGBA(src)
	d := premulRGBA(dst)
	switch mode {
	case colrCompositeClear:
		return color.RGBA{}
	case colrCompositeSrc:
		return src
	case colrCompositeDest:
		return dst
	case colrCompositeSrcOver:
		return unpremulRGBA(addPremul(s, scalePremul(d, 1-s.a)))
	case colrCompositeDestOver:
		return unpremulRGBA(addPremul(d, scalePremul(s, 1-d.a)))
	case colrCompositeSrcIn:
		return unpremulRGBA(scalePremul(s, d.a))
	case colrCompositeDestIn:
		return unpremulRGBA(scalePremul(d, s.a))
	case colrCompositeSrcOut:
		return unpremulRGBA(scalePremul(s, 1-d.a))
	case colrCompositeDestOut:
		return unpremulRGBA(scalePremul(d, 1-s.a))
	case colrCompositeSrcAtop:
		return unpremulRGBA(addPremul(scalePremul(s, d.a), scalePremul(d, 1-s.a)))
	case colrCompositeDestAtop:
		return unpremulRGBA(addPremul(scalePremul(d, s.a), scalePremul(s, 1-d.a)))
	case colrCompositeXor:
		return unpremulRGBA(addPremul(scalePremul(s, 1-d.a), scalePremul(d, 1-s.a)))
	case colrCompositePlus:
		return unpremulRGBA(premul{r: math.Min(1, s.r+d.r), g: math.Min(1, s.g+d.g), b: math.Min(1, s.b+d.b), a: math.Min(1, s.a+d.a)})
	case colrCompositeScreen, colrCompositeOverlay, colrCompositeDarken, colrCompositeLighten, colrCompositeColorDodge, colrCompositeColorBurn, colrCompositeHardLight, colrCompositeSoftLight, colrCompositeDifference, colrCompositeExclusion, colrCompositeMultiply, colrCompositeHSLHue, colrCompositeHSLSaturation, colrCompositeHSLColor, colrCompositeHSLLuminosity:
		return blendCOLRPixel(src, dst, mode)
	default:
		return color.RGBA{}
	}
}

type premul struct{ r, g, b, a float64 }

func premulRGBA(c color.RGBA) premul {
	a := float64(c.A) / 255
	return premul{r: float64(c.R) / 255 * a, g: float64(c.G) / 255 * a, b: float64(c.B) / 255 * a, a: a}
}

func unpremulRGBA(p premul) color.RGBA {
	p.a = clampUnit(p.a)
	if p.a == 0 {
		return color.RGBA{}
	}
	return color.RGBA{R: byte255(p.r / p.a), G: byte255(p.g / p.a), B: byte255(p.b / p.a), A: byte255(p.a)}
}

func addPremul(a, b premul) premul {
	return premul{r: a.r + b.r, g: a.g + b.g, b: a.b + b.b, a: a.a + b.a}
}
func scalePremul(p premul, f float64) premul {
	return premul{r: p.r * f, g: p.g * f, b: p.b * f, a: p.a * f}
}
func byte255(v float64) uint8     { return uint8(math.Round(clampUnit(v) * 255)) }
func clampUnit(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func blendCOLRPixel(src, dst color.RGBA, mode int) color.RGBA {
	as := float64(src.A) / 255
	ab := float64(dst.A) / 255
	a := as + ab - as*ab
	if a == 0 {
		return color.RGBA{}
	}
	cs := [3]float64{float64(src.R) / 255, float64(src.G) / 255, float64(src.B) / 255}
	cb := [3]float64{float64(dst.R) / 255, float64(dst.G) / 255, float64(dst.B) / 255}
	blendedColor := blendColor(cs, cb, mode)
	var out [3]float64
	for i := 0; i < 3; i++ {
		premul := (1-ab)*as*cs[i] + (1-as)*ab*cb[i] + as*ab*blendedColor[i]
		out[i] = premul / a
	}
	return color.RGBA{R: byte255(out[0]), G: byte255(out[1]), B: byte255(out[2]), A: byte255(a)}
}

func blendChannel(s, b float64, mode int) float64 {
	switch mode {
	case colrCompositeScreen:
		return b + s - b*s
	case colrCompositeOverlay:
		return hardLightChannel(b, s)
	case colrCompositeDarken:
		return math.Min(b, s)
	case colrCompositeLighten:
		return math.Max(b, s)
	case colrCompositeColorDodge:
		if s >= 1 {
			return 1
		}
		return math.Min(1, b/(1-s))
	case colrCompositeColorBurn:
		if s <= 0 {
			return 0
		}
		return 1 - math.Min(1, (1-b)/s)
	case colrCompositeHardLight:
		return hardLightChannel(s, b)
	case colrCompositeSoftLight:
		if s <= 0.5 {
			return b - (1-2*s)*b*(1-b)
		}
		d := math.Sqrt(b)
		return b + (2*s-1)*(d-b)
	case colrCompositeDifference:
		return math.Abs(b - s)
	case colrCompositeExclusion:
		return b + s - 2*b*s
	case colrCompositeMultiply:
		return b * s
	default:
		return s
	}
}

func blendColor(s, b [3]float64, mode int) [3]float64 {
	switch mode {
	case colrCompositeHSLHue:
		return setLum(setSat(s, sat(b)), lum(b))
	case colrCompositeHSLSaturation:
		return setLum(setSat(b, sat(s)), lum(b))
	case colrCompositeHSLColor:
		return setLum(s, lum(b))
	case colrCompositeHSLLuminosity:
		return setLum(b, lum(s))
	default:
		return [3]float64{blendChannel(s[0], b[0], mode), blendChannel(s[1], b[1], mode), blendChannel(s[2], b[2], mode)}
	}
}

func lum(c [3]float64) float64 { return 0.3*c[0] + 0.59*c[1] + 0.11*c[2] }

func sat(c [3]float64) float64 {
	return math.Max(c[0], math.Max(c[1], c[2])) - math.Min(c[0], math.Min(c[1], c[2]))
}

func setLum(c [3]float64, target float64) [3]float64 {
	delta := target - lum(c)
	return clipColor([3]float64{c[0] + delta, c[1] + delta, c[2] + delta})
}

func clipColor(c [3]float64) [3]float64 {
	l := lum(c)
	n := math.Min(c[0], math.Min(c[1], c[2]))
	x := math.Max(c[0], math.Max(c[1], c[2]))
	if n < 0 {
		for i := 0; i < 3; i++ {
			c[i] = l + ((c[i]-l)*l)/(l-n)
		}
	}
	if x > 1 {
		for i := 0; i < 3; i++ {
			c[i] = l + ((c[i]-l)*(1-l))/(x-l)
		}
	}
	return c
}

func setSat(c [3]float64, target float64) [3]float64 {
	minI, midI, maxI := sortedColorIndexes(c)
	if c[maxI] > c[minI] {
		c[midI] = ((c[midI] - c[minI]) * target) / (c[maxI] - c[minI])
		c[maxI] = target
	} else {
		c[midI] = 0
		c[maxI] = 0
	}
	c[minI] = 0
	return c
}

func sortedColorIndexes(c [3]float64) (int, int, int) {
	idx := [3]int{0, 1, 2}
	for i := 0; i < len(idx); i++ {
		for j := i + 1; j < len(idx); j++ {
			if c[idx[j]] < c[idx[i]] {
				idx[i], idx[j] = idx[j], idx[i]
			}
		}
	}
	return idx[0], idx[1], idx[2]
}

func hardLightChannel(s, b float64) float64 {
	if s <= 0.5 {
		return 2 * s * b
	}
	return 1 - 2*(1-s)*(1-b)
}

func pointXY(p fixed.Point26_6, offset int, baseline int, transform COLRTransform) (float32, float32) {
	x, y := transform.Apply(float64(p.X)/64, float64(p.Y)/64)
	return float32(offset) + float32(x), float32(baseline) + float32(y)
}
