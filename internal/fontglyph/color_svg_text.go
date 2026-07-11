package fontglyph

import (
	"encoding/xml"
	"image"
	"image/color"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const svgBasicTextBaseline = 11
const svgBasicTextHeight = 13

func drawSVGText(img *image.RGBA, dec *xml.Decoder, el xml.StartElement, viewport svgViewport, gradients map[string]svgLinearGradient) bool {
	paint, ok := parseSVGPaint(el, gradients)
	if !ok || paint.linear != nil {
		return false
	}
	text := collectSVGText(dec, el)
	if text == "" {
		return false
	}
	x, _ := parseSVGNumber(attr(el, "x"))
	fontSize, okSize := parseSVGNumber(attr(el, "font-size"))
	if !okSize || fontSize <= 0 {
		fontSize = 13
	}
	y, okY := parseSVGNumber(attr(el, "y"))
	if !okY {
		y = fontSize
	}
	px, py := svgPointToPixelFloat(img, viewport, x, y)
	scale := svgTextScale(img, viewport, fontSize)
	mask := rasterizeBasicSVGTextMask(text)
	if mask.Bounds().Empty() {
		return false
	}
	destX := px
	width := float64(mask.Bounds().Dx()) * scale
	switch strings.ToLower(strings.TrimSpace(attr(el, "text-anchor"))) {
	case "middle":
		destX -= width / 2
	case "end":
		destX -= width
	}
	destY := py - svgBasicTextBaseline*scale
	switch strings.ToLower(strings.TrimSpace(attr(el, "dominant-baseline"))) {
	case "middle", "central":
		destY = py - (svgBasicTextHeight*scale)/2
	case "hanging", "text-before-edge":
		destY = py
	}
	return blitScaledTextMask(img, mask, destX, destY, scale, paint.solid)
}

func collectSVGText(dec *xml.Decoder, el xml.StartElement) string {
	var b strings.Builder
	for {
		token, err := dec.Token()
		if err != nil {
			break
		}
		switch token := token.(type) {
		case xml.CharData:
			b.Write([]byte(token))
		case xml.StartElement:
			if token.Name.Local == "tspan" {
				b.WriteString(collectSVGText(dec, token))
			} else {
				skipSVGElement(dec, token.Name.Local)
			}
		case xml.EndElement:
			if token.Name.Local == el.Name.Local {
				return normalizeSVGText(b.String())
			}
		}
	}
	return normalizeSVGText(b.String())
}

func skipSVGElement(dec *xml.Decoder, localName string) {
	depth := 1
	for depth > 0 {
		token, err := dec.Token()
		if err != nil {
			return
		}
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Local == localName {
				depth++
			}
		case xml.EndElement:
			if token.Name.Local == localName {
				depth--
			}
		}
	}
}

func normalizeSVGText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func svgTextScale(img *image.RGBA, viewport svgViewport, fontSize float64) float64 {
	scaleX := float64(img.Bounds().Dx()) / viewport.width
	scaleY := float64(img.Bounds().Dy()) / viewport.height
	scale := fontSize * math.Min(scaleX, scaleY) / svgBasicTextHeight
	if scale < 0.25 {
		return 0.25
	}
	return scale
}

func rasterizeBasicSVGTextMask(text string) *image.RGBA {
	face := basicfont.Face7x13
	d := font.Drawer{Face: face}
	width := d.MeasureString(text).Ceil()
	if width <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	mask := image.NewRGBA(image.Rect(0, 0, width, svgBasicTextHeight))
	d = font.Drawer{
		Dst:  mask,
		Src:  image.NewUniform(color.RGBA{A: 255}),
		Face: face,
		Dot:  fixed.P(0, svgBasicTextBaseline),
	}
	d.DrawString(text)
	return mask
}

func blitScaledTextMask(dst *image.RGBA, mask *image.RGBA, destX, destY, scale float64, fill color.RGBA) bool {
	painted := false
	bounds := dst.Bounds()
	for sy := mask.Bounds().Min.Y; sy < mask.Bounds().Max.Y; sy++ {
		for sx := mask.Bounds().Min.X; sx < mask.Bounds().Max.X; sx++ {
			alpha := mask.RGBAAt(sx, sy).A
			if alpha == 0 {
				continue
			}
			x0 := int(math.Floor(destX + float64(sx)*scale))
			y0 := int(math.Floor(destY + float64(sy)*scale))
			x1 := int(math.Ceil(destX + float64(sx+1)*scale))
			y1 := int(math.Ceil(destY + float64(sy+1)*scale))
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					if !image.Pt(x, y).In(bounds) {
						continue
					}
					c := fill
					c.A = uint8((uint16(c.A)*uint16(alpha) + 127) / 255)
					overRGBA(dst, x, y, c)
					painted = true
				}
			}
		}
	}
	return painted
}
