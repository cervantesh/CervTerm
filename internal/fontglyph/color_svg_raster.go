package fontglyph

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
)

type svgViewport struct {
	minX   float64
	minY   float64
	width  float64
	height float64
}

func rasterizeSVGDocument(doc []byte, width int, height int) (*image.RGBA, bool) {
	if width <= 0 || height <= 0 || len(doc) == 0 {
		return nil, false
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	viewport := svgViewport{width: float64(width), height: float64(height)}
	gradients := make(map[string]svgLinearGradient)
	dec := xml.NewDecoder(bytes.NewReader(doc))
	painted := false
	for {
		token, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "svg":
			viewport = parseSVGViewport(start, viewport)
		case "linearGradient":
			if id, gradient, ok := parseSVGLinearGradient(dec, start); ok {
				gradients[id] = gradient
			}
		case "rect":
			if drawSVGRect(img, start, viewport, gradients) {
				painted = true
			}
		case "circle":
			if drawSVGCircle(img, start, viewport, gradients) {
				painted = true
			}
		case "path":
			if drawSVGPath(img, start, viewport, gradients) {
				painted = true
			}
		case "text":
			if drawSVGText(img, dec, start, viewport, gradients) {
				painted = true
			}
		}
	}
	return img, painted
}

func parseSVGViewport(el xml.StartElement, fallback svgViewport) svgViewport {
	viewport := fallback
	if viewBox := attr(el, "viewBox"); viewBox != "" {
		parts := strings.Fields(strings.ReplaceAll(viewBox, ",", " "))
		if len(parts) == 4 {
			if minX, ok := parseSVGNumber(parts[0]); ok {
				if minY, ok := parseSVGNumber(parts[1]); ok {
					if w, ok := parseSVGNumber(parts[2]); ok && w > 0 {
						if h, ok := parseSVGNumber(parts[3]); ok && h > 0 {
							return svgViewport{minX: minX, minY: minY, width: w, height: h}
						}
					}
				}
			}
		}
	}
	if w, ok := parseSVGNumber(attr(el, "width")); ok && w > 0 {
		viewport.width = w
	}
	if h, ok := parseSVGNumber(attr(el, "height")); ok && h > 0 {
		viewport.height = h
	}
	return viewport
}

func drawSVGRect(img *image.RGBA, el xml.StartElement, viewport svgViewport, gradients map[string]svgLinearGradient) bool {
	paint, ok := parseSVGPaint(el, gradients)
	if !ok {
		return false
	}
	x, _ := parseSVGNumber(attr(el, "x"))
	y, _ := parseSVGNumber(attr(el, "y"))
	w, okW := parseSVGNumber(attr(el, "width"))
	h, okH := parseSVGNumber(attr(el, "height"))
	if !okW || !okH || w <= 0 || h <= 0 {
		return false
	}
	x0, y0 := svgPointToPixel(img, viewport, x, y)
	x1, y1 := svgPointToPixel(img, viewport, x+w, y+h)
	rect := image.Rect(svgMinInt(x0, x1), svgMinInt(y0, y1), svgMaxInt(x0, x1), svgMaxInt(y0, y1)).Intersect(img.Bounds())
	if rect.Empty() {
		return false
	}
	for py := rect.Min.Y; py < rect.Max.Y; py++ {
		for px := rect.Min.X; px < rect.Max.X; px++ {
			ux, uy := svgPixelToPointFloat(img, viewport, px, py)
			if c, ok := paint.colorAt(ux, uy, viewport); ok {
				overRGBA(img, px, py, c)
			}
		}
	}
	return true
}

func drawSVGCircle(img *image.RGBA, el xml.StartElement, viewport svgViewport, gradients map[string]svgLinearGradient) bool {
	paint, ok := parseSVGPaint(el, gradients)
	if !ok {
		return false
	}
	cx, okX := parseSVGNumber(attr(el, "cx"))
	cy, okY := parseSVGNumber(attr(el, "cy"))
	r, okR := parseSVGNumber(attr(el, "r"))
	if !okX || !okY || !okR || r <= 0 {
		return false
	}
	centerX, centerY := svgPointToPixelFloat(img, viewport, cx, cy)
	scaleX := float64(img.Bounds().Dx()) / viewport.width
	scaleY := float64(img.Bounds().Dy()) / viewport.height
	radius := r * math.Min(scaleX, scaleY)
	rect := image.Rect(int(math.Floor(centerX-radius)), int(math.Floor(centerY-radius)), int(math.Ceil(centerX+radius)), int(math.Ceil(centerY+radius))).Intersect(img.Bounds())
	if rect.Empty() {
		return false
	}
	r2 := radius * radius
	for py := rect.Min.Y; py < rect.Max.Y; py++ {
		for px := rect.Min.X; px < rect.Max.X; px++ {
			dx := float64(px) + 0.5 - centerX
			dy := float64(py) + 0.5 - centerY
			if dx*dx+dy*dy <= r2 {
				ux, uy := svgPixelToPointFloat(img, viewport, px, py)
				if c, ok := paint.colorAt(ux, uy, viewport); ok {
					overRGBA(img, px, py, c)
				}
			}
		}
	}
	return true
}

func parseSVGFill(el xml.StartElement) (color.RGBA, bool) {
	fill := attr(el, "fill")
	if fill == "" {
		fill = attr(el, "color")
	}
	c, ok := parseSVGColor(fill)
	if !ok {
		return color.RGBA{}, false
	}
	if opacity, ok := parseSVGNumber(attr(el, "opacity")); ok {
		c.A = uint8(math.Round(float64(c.A) * clamp01(opacity)))
	}
	if opacity, ok := parseSVGNumber(attr(el, "fill-opacity")); ok {
		c.A = uint8(math.Round(float64(c.A) * clamp01(opacity)))
	}
	return c, c.A > 0
}

func parseSVGColor(value string) (color.RGBA, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "none" {
		return color.RGBA{}, false
	}
	if strings.HasPrefix(value, "#") {
		hex := strings.TrimPrefix(value, "#")
		if len(hex) == 3 {
			r, okR := parseHexByte(strings.Repeat(hex[0:1], 2))
			g, okG := parseHexByte(strings.Repeat(hex[1:2], 2))
			b, okB := parseHexByte(strings.Repeat(hex[2:3], 2))
			return color.RGBA{R: r, G: g, B: b, A: 255}, okR && okG && okB
		}
		if len(hex) == 6 {
			r, okR := parseHexByte(hex[0:2])
			g, okG := parseHexByte(hex[2:4])
			b, okB := parseHexByte(hex[4:6])
			return color.RGBA{R: r, G: g, B: b, A: 255}, okR && okG && okB
		}
	}
	switch value {
	case "black":
		return color.RGBA{A: 255}, true
	case "white":
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}, true
	case "red":
		return color.RGBA{R: 255, A: 255}, true
	case "green":
		return color.RGBA{G: 128, A: 255}, true
	case "lime":
		return color.RGBA{G: 255, A: 255}, true
	case "blue":
		return color.RGBA{B: 255, A: 255}, true
	}
	return color.RGBA{}, false
}

func parseHexByte(value string) (uint8, bool) {
	parsed, err := strconv.ParseUint(value, 16, 8)
	if err != nil {
		return 0, false
	}
	return uint8(parsed), true
}

func parseSVGNumber(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	value = strings.TrimSuffix(value, "px")
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func svgPointToPixel(img *image.RGBA, viewport svgViewport, x, y float64) (int, int) {
	px, py := svgPointToPixelFloat(img, viewport, x, y)
	return int(math.Round(px)), int(math.Round(py))
}

func svgPointToPixelFloat(img *image.RGBA, viewport svgViewport, x, y float64) (float64, float64) {
	scaleX := float64(img.Bounds().Dx()) / viewport.width
	scaleY := float64(img.Bounds().Dy()) / viewport.height
	return (x - viewport.minX) * scaleX, (y - viewport.minY) * scaleY
}

func svgPixelToPointFloat(img *image.RGBA, viewport svgViewport, px int, py int) (float64, float64) {
	scaleX := viewport.width / float64(img.Bounds().Dx())
	scaleY := viewport.height / float64(img.Bounds().Dy())
	return viewport.minX + (float64(px)+0.5)*scaleX, viewport.minY + (float64(py)+0.5)*scaleY
}

func attr(el xml.StartElement, name string) string {
	for _, attr := range el.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func svgMinInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func svgMaxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
