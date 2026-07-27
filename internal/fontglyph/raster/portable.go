package raster

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// RasterizeRuneValue is RasterizeRune with the source rune kept explicit.
func RasterizeRuneValue(face font.Face, value rune, bounds fixed.Rectangle26_6, advance fixed.Int26_6, metrics Metrics, cellSpan int) Glyph {
	cellSpan = max(1, cellSpan)
	img := image.NewRGBA(image.Rect(0, 0, metrics.CellWidth*cellSpan, metrics.CellHeight))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	drawer := &font.Drawer{Dst: img, Src: image.NewUniform(color.RGBA{255, 255, 255, 255}), Face: face, Dot: fixed.Point26_6{X: (fixed.I(metrics.CellWidth*cellSpan) - advance) / 2, Y: fixed.I(metrics.Baseline)}}
	drawer.DrawString(string(value))
	return Glyph{Image: img, Width: (bounds.Max.X - bounds.Min.X).Ceil(), Height: (bounds.Max.Y - bounds.Min.Y).Ceil(), BearingX: bounds.Min.X.Ceil(), BearingY: -bounds.Min.Y.Ceil(), AdvanceX: float64(advance) / 64, CellSpan: cellSpan}
}

// RasterizeCluster renders one portable fallback cluster.
func RasterizeCluster(face font.Face, cluster string, metrics Metrics, cellSpan int) Glyph {
	cellSpan = max(1, cellSpan)
	img := image.NewRGBA(image.Rect(0, 0, metrics.CellWidth*cellSpan, metrics.CellHeight))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	drawer := &font.Drawer{Dst: img, Src: image.NewUniform(color.RGBA{255, 255, 255, 255}), Face: face, Dot: fixed.Point26_6{X: fixed.I(1), Y: fixed.I(metrics.Baseline)}}
	drawer.DrawString(cluster)
	advance := drawer.MeasureString(cluster)
	return Glyph{Image: img, Width: max(1, advance.Ceil()), Height: metrics.CellHeight, BearingX: 1, BearingY: metrics.Baseline, AdvanceX: float64(advance) / 64, CellSpan: cellSpan}
}

// RasterizeBitmap fits one bitmap strike while preserving historical origin placement.
func RasterizeBitmap(bitmap BitmapGlyph, metrics Metrics, cellSpan int, advance fixed.Int26_6) (Glyph, bool) {
	cellSpan = max(1, cellSpan)
	canvasW, canvasH := metrics.CellWidth*cellSpan, metrics.CellHeight
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	srcBounds := bitmap.Image.Bounds()
	if srcBounds.Dx() <= 0 || srcBounds.Dy() <= 0 {
		return Glyph{}, false
	}
	scale := math.Min(float64(canvasW)/float64(srcBounds.Dx()), float64(canvasH)/float64(srcBounds.Dy()))
	if scale <= 0 {
		return Glyph{}, false
	}
	dstW := max(1, int(math.Round(float64(srcBounds.Dx())*scale)))
	dstH := max(1, int(math.Round(float64(srcBounds.Dy())*scale)))
	dstX := (canvasW - dstW) / 2
	dstY := metrics.Baseline - int(math.Round(float64(bitmap.OriginOffsetY)*scale))
	if dstY < 0 || dstY+dstH > canvasH {
		dstY = (canvasH - dstH) / 2
	}
	xdraw.CatmullRom.Scale(img, image.Rect(dstX, dstY, dstX+dstW, dstY+dstH), bitmap.Image, srcBounds, xdraw.Over, nil)
	return Glyph{Image: img, Width: dstW, Height: dstH, BearingX: dstX, BearingY: canvasH - dstY, AdvanceX: float64(advance) / 64, CellSpan: cellSpan, HasColor: true}, true
}

// RasterizeShapedBitmaps fits one or more bitmap glyphs in shaped order.
func RasterizeShapedBitmaps(bitmaps []BitmapGlyph, shaped []ShapeGlyph, metrics Metrics, cellSpan int) (Glyph, bool) {
	if len(bitmaps) == 0 || len(bitmaps) != len(shaped) {
		return Glyph{}, false
	}
	cellSpan = max(1, cellSpan)
	canvasW, canvasH := metrics.CellWidth*cellSpan, metrics.CellHeight
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	advance := 0.0
	if len(bitmaps) == 1 {
		if !drawBitmapGlyphFit(img, bitmaps[0], image.Rect(0, 0, canvasW, canvasH)) {
			return Glyph{}, false
		}
		advance = shaped[0].XAdvance
	} else {
		slotWidth := max(1, canvasW/len(bitmaps))
		for i, bitmap := range bitmaps {
			slot := image.Rect(i*slotWidth, 0, canvasW, canvasH)
			if i < len(bitmaps)-1 {
				slot.Max.X = (i + 1) * slotWidth
			}
			if !drawBitmapGlyphFit(img, bitmap, slot) {
				return Glyph{}, false
			}
			advance += shaped[i].XAdvance
		}
	}
	if advance <= 0 {
		advance = float64(canvasW)
	}
	if !HasVisibleRGBA(img) {
		return Glyph{}, false
	}
	return Glyph{Image: img, Width: canvasW, Height: canvasH, BearingY: canvasH, AdvanceX: advance, CellSpan: cellSpan, HasColor: true}, true
}

func drawBitmapGlyphFit(dst *image.RGBA, bitmap BitmapGlyph, slot image.Rectangle) bool {
	source := bitmap.Image.Bounds()
	if source.Dx() <= 0 || source.Dy() <= 0 || slot.Dx() <= 0 || slot.Dy() <= 0 {
		return false
	}
	scale := math.Min(float64(slot.Dx())/float64(source.Dx()), float64(slot.Dy())/float64(source.Dy()))
	if scale <= 0 {
		return false
	}
	width := max(1, int(math.Round(float64(source.Dx())*scale)))
	height := max(1, int(math.Round(float64(source.Dy())*scale)))
	x := slot.Min.X + (slot.Dx()-width)/2
	y := slot.Min.Y + (slot.Dy()-height)/2
	xdraw.CatmullRom.Scale(dst, image.Rect(x, y, x+width, y+height), bitmap.Image, source, xdraw.Over, nil)
	return true
}

var subpixelFIRKernel = [...]int{1, 2, 3, 2, 1}

// RasterizeSubpixel renders one outline at three horizontal samples per pixel.
func RasterizeSubpixel(parsed *sfnt.Font, glyphID sfnt.GlyphIndex, bounds fixed.Rectangle26_6, metrics Metrics, cellSpan int) (*image.RGBA, bool) {
	if parsed == nil {
		return nil, false
	}
	var buffer sfnt.Buffer
	segments, err := parsed.LoadGlyph(&buffer, glyphID, fixed.I(int(metrics.PPEM)), nil)
	if err != nil {
		return nil, false
	}
	width := metrics.CellWidth * max(1, cellSpan)
	highWidth := 3 * width
	mask := image.NewAlpha(image.Rect(0, 0, highWidth, metrics.CellHeight))
	rasterizeSubpixelSegments(mask, segments, bounds, metrics.Baseline)
	out := image.NewRGBA(image.Rect(0, 0, width, metrics.CellHeight))
	row := make([]uint8, highWidth)
	for y := 0; y < metrics.CellHeight; y++ {
		for x := range highWidth {
			row[x] = mask.AlphaAt(x, y).A
		}
		for x := 0; x < width; x++ {
			r := applySubpixelFIR(row, 3*x)
			g := applySubpixelFIR(row, 3*x+1)
			b := applySubpixelFIR(row, 3*x+2)
			a := uint8((int(r) + int(g) + int(b) + 1) / 3)
			out.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	return out, true
}

// ApplySubpixelFIR exposes the deterministic five-tap LCD filter.
func ApplySubpixelFIR(samples []uint8, index int) uint8 { return applySubpixelFIR(samples, index) }

func applySubpixelFIR(samples []uint8, index int) uint8 {
	if len(samples) == 0 {
		return 0
	}
	sum := 0
	for tap, weight := range subpixelFIRKernel {
		i := max(0, min(len(samples)-1, index+tap-len(subpixelFIRKernel)/2))
		sum += int(samples[i]) * weight
	}
	return uint8((sum + 4) / 9)
}

func rasterizeSubpixelSegments(dst *image.Alpha, segments sfnt.Segments, bounds fixed.Rectangle26_6, baseline int) {
	rasterizer := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
	for _, segment := range segments {
		xy := func(i int) (float32, float32) {
			point := segment.Args[i]
			return 3 * (1 + float32(point.X-bounds.Min.X)/64), float32(baseline) + float32(point.Y)/64
		}
		switch segment.Op {
		case sfnt.SegmentOpMoveTo:
			x, y := xy(0)
			rasterizer.MoveTo(x, y)
		case sfnt.SegmentOpLineTo:
			x, y := xy(0)
			rasterizer.LineTo(x, y)
		case sfnt.SegmentOpQuadTo:
			bx, by := xy(0)
			cx, cy := xy(1)
			rasterizer.QuadTo(bx, by, cx, cy)
		case sfnt.SegmentOpCubeTo:
			bx, by := xy(0)
			cx, cy := xy(1)
			dx, dy := xy(2)
			rasterizer.CubeTo(bx, by, cx, cy, dx, dy)
		}
	}
	rasterizer.Draw(dst, dst.Bounds(), image.NewUniform(color.Alpha{A: 255}), image.Point{})
}
