package fontglyph

import (
	"encoding/xml"
	"image/color"
	"math"
	"strings"
)

type svgPaint struct {
	solid  color.RGBA
	linear *svgLinearGradient
}

type svgLinearGradient struct {
	x1    svgLength
	y1    svgLength
	x2    svgLength
	y2    svgLength
	stops []svgGradientStop
}

type svgGradientStop struct {
	offset float64
	color  color.RGBA
}

type svgLength struct {
	value   float64
	percent bool
}

func parseSVGLinearGradient(dec *xml.Decoder, el xml.StartElement) (string, svgLinearGradient, bool) {
	id := attr(el, "id")
	if id == "" {
		return "", svgLinearGradient{}, false
	}
	gradient := svgLinearGradient{
		x1: parseSVGLengthDefault(attr(el, "x1"), 0, true),
		y1: parseSVGLengthDefault(attr(el, "y1"), 0, true),
		x2: parseSVGLengthDefault(attr(el, "x2"), 1, true),
		y2: parseSVGLengthDefault(attr(el, "y2"), 0, true),
	}
	for {
		token, err := dec.Token()
		if err != nil {
			break
		}
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Local == "stop" {
				if stop, ok := parseSVGGradientStop(token); ok {
					gradient.stops = append(gradient.stops, stop)
				}
			}
		case xml.EndElement:
			if token.Name.Local == el.Name.Local {
				return id, gradient, len(gradient.stops) > 0
			}
		}
	}
	return id, gradient, len(gradient.stops) > 0
}

func parseSVGPaint(el xml.StartElement, gradients map[string]svgLinearGradient) (svgPaint, bool) {
	fill := attr(el, "fill")
	if fill == "" {
		fill = attr(el, "color")
	}
	fill = strings.TrimSpace(fill)
	if strings.HasPrefix(fill, "url(#") && strings.HasSuffix(fill, ")") {
		id := strings.TrimSuffix(strings.TrimPrefix(fill, "url(#"), ")")
		if gradient, ok := gradients[id]; ok {
			return svgPaint{linear: &gradient}, true
		}
		return svgPaint{}, false
	}
	c, ok := parseSVGColor(fill)
	if !ok {
		return svgPaint{}, false
	}
	c = applySVGOpacity(c, el)
	return svgPaint{solid: c}, c.A > 0
}

func parseSVGGradientStop(el xml.StartElement) (svgGradientStop, bool) {
	offset, ok := parseSVGOffset(attr(el, "offset"))
	if !ok {
		return svgGradientStop{}, false
	}
	c, ok := parseSVGColor(attr(el, "stop-color"))
	if !ok {
		return svgGradientStop{}, false
	}
	if opacity, ok := parseSVGNumber(attr(el, "stop-opacity")); ok {
		c.A = uint8(math.Round(float64(c.A) * clamp01(opacity)))
	}
	return svgGradientStop{offset: clamp01(offset), color: c}, true
}

func parseSVGOffset(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "%") {
		parsed, ok := parseSVGNumber(strings.TrimSuffix(value, "%"))
		return parsed / 100, ok
	}
	return parseSVGNumber(value)
}

func parseSVGLengthDefault(value string, fallback float64, percent bool) svgLength {
	value = strings.TrimSpace(value)
	if value == "" {
		return svgLength{value: fallback, percent: percent}
	}
	if strings.HasSuffix(value, "%") {
		parsed, ok := parseSVGNumber(strings.TrimSuffix(value, "%"))
		if ok {
			return svgLength{value: parsed / 100, percent: true}
		}
	}
	parsed, ok := parseSVGNumber(value)
	if !ok {
		return svgLength{value: fallback, percent: percent}
	}
	return svgLength{value: parsed}
}

func (l svgLength) resolve(min float64, size float64) float64 {
	if l.percent {
		return min + l.value*size
	}
	return l.value
}

func (p svgPaint) colorAt(x float64, y float64, viewport svgViewport) (color.RGBA, bool) {
	if p.linear == nil {
		return p.solid, p.solid.A > 0
	}
	return p.linear.colorAt(x, y, viewport)
}

func (g svgLinearGradient) colorAt(x float64, y float64, viewport svgViewport) (color.RGBA, bool) {
	if len(g.stops) == 0 {
		return color.RGBA{}, false
	}
	x1 := g.x1.resolve(viewport.minX, viewport.width)
	y1 := g.y1.resolve(viewport.minY, viewport.height)
	x2 := g.x2.resolve(viewport.minX, viewport.width)
	y2 := g.y2.resolve(viewport.minY, viewport.height)
	dx, dy := x2-x1, y2-y1
	denom := dx*dx + dy*dy
	t := 0.0
	if denom > 0 {
		t = ((x-x1)*dx + (y-y1)*dy) / denom
	}
	t = clamp01(t)
	prev := g.stops[0]
	for _, next := range g.stops[1:] {
		if t <= next.offset {
			span := next.offset - prev.offset
			local := 0.0
			if span > 0 {
				local = (t - prev.offset) / span
			}
			return lerpSVGRGBA(prev.color, next.color, clamp01(local)), true
		}
		prev = next
	}
	return g.stops[len(g.stops)-1].color, true
}

func applySVGOpacity(c color.RGBA, el xml.StartElement) color.RGBA {
	if opacity, ok := parseSVGNumber(attr(el, "opacity")); ok {
		c.A = uint8(math.Round(float64(c.A) * clamp01(opacity)))
	}
	if opacity, ok := parseSVGNumber(attr(el, "fill-opacity")); ok {
		c.A = uint8(math.Round(float64(c.A) * clamp01(opacity)))
	}
	return c
}

func lerpSVGRGBA(a color.RGBA, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(math.Round(float64(a.R) + (float64(b.R)-float64(a.R))*t)),
		G: uint8(math.Round(float64(a.G) + (float64(b.G)-float64(a.G))*t)),
		B: uint8(math.Round(float64(a.B) + (float64(b.B)-float64(a.B))*t)),
		A: uint8(math.Round(float64(a.A) + (float64(b.A)-float64(a.A))*t)),
	}
}
