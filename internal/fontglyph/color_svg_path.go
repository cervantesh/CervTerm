package fontglyph

import (
	"encoding/xml"
	"image"
	"math"
	"strings"
)

const svgCurveSteps = 16

type svgPoint struct {
	x float64
	y float64
}

func drawSVGPath(img *image.RGBA, el xml.StartElement, viewport svgViewport, gradients map[string]svgLinearGradient) bool {
	paint, ok := parseSVGPaint(el, gradients)
	if !ok {
		return false
	}
	points, ok := parseSVGPathPolygon(attr(el, "d"))
	if !ok || len(points) < 3 {
		return false
	}
	pixels := make([]svgPoint, len(points))
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	for i, point := range points {
		x, y := svgPointToPixelFloat(img, viewport, point.x, point.y)
		pixels[i] = svgPoint{x: x, y: y}
		minX = math.Min(minX, x)
		minY = math.Min(minY, y)
		maxX = math.Max(maxX, x)
		maxY = math.Max(maxY, y)
	}
	rect := image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX)), int(math.Ceil(maxY))).Intersect(img.Bounds())
	if rect.Empty() {
		return false
	}
	for py := rect.Min.Y; py < rect.Max.Y; py++ {
		for px := rect.Min.X; px < rect.Max.X; px++ {
			if svgPointInPolygon(float64(px)+0.5, float64(py)+0.5, pixels) {
				ux, uy := svgPixelToPointFloat(img, viewport, px, py)
				if c, ok := paint.colorAt(ux, uy, viewport); ok {
					overRGBA(img, px, py, c)
				}
			}
		}
	}
	return true
}

func parseSVGPathPolygon(d string) ([]svgPoint, bool) {
	tokens := svgPathTokens(d)
	if len(tokens) == 0 {
		return nil, false
	}
	var points []svgPoint
	var cursor svgPoint
	var start svgPoint
	cmd := ""
	for i := 0; i < len(tokens); {
		if isSVGPathCommand(tokens[i]) {
			cmd = tokens[i]
			i++
			if cmd == "Z" || cmd == "z" {
				cursor = start
				continue
			}
		}
		switch cmd {
		case "M", "L":
			point, next, ok := readSVGPoint(tokens, i)
			if !ok {
				return nil, false
			}
			cursor = point
			if cmd == "M" && len(points) == 0 {
				start = cursor
			}
			points = append(points, cursor)
			i = next
			if cmd == "M" {
				cmd = "L"
			}
		case "m", "l":
			delta, next, ok := readSVGPoint(tokens, i)
			if !ok {
				return nil, false
			}
			cursor = svgPoint{x: cursor.x + delta.x, y: cursor.y + delta.y}
			if cmd == "m" && len(points) == 0 {
				start = cursor
			}
			points = append(points, cursor)
			i = next
			if cmd == "m" {
				cmd = "l"
			}
		case "H", "h":
			value, next, ok := readSVGNumberToken(tokens, i)
			if !ok {
				return nil, false
			}
			if cmd == "h" {
				cursor.x += value
			} else {
				cursor.x = value
			}
			points = append(points, cursor)
			i = next
		case "V", "v":
			value, next, ok := readSVGNumberToken(tokens, i)
			if !ok {
				return nil, false
			}
			if cmd == "v" {
				cursor.y += value
			} else {
				cursor.y = value
			}
			points = append(points, cursor)
			i = next
		case "Q", "q":
			control, end, next, ok := readSVGQuadratic(tokens, i)
			if !ok {
				return nil, false
			}
			if cmd == "q" {
				control = svgPoint{x: cursor.x + control.x, y: cursor.y + control.y}
				end = svgPoint{x: cursor.x + end.x, y: cursor.y + end.y}
			}
			points = appendQuadratic(points, cursor, control, end)
			cursor = end
			i = next
		case "C", "c":
			c1, c2, end, next, ok := readSVGCubic(tokens, i)
			if !ok {
				return nil, false
			}
			if cmd == "c" {
				c1 = svgPoint{x: cursor.x + c1.x, y: cursor.y + c1.y}
				c2 = svgPoint{x: cursor.x + c2.x, y: cursor.y + c2.y}
				end = svgPoint{x: cursor.x + end.x, y: cursor.y + end.y}
			}
			points = appendCubic(points, cursor, c1, c2, end)
			cursor = end
			i = next
		default:
			return nil, false
		}
	}
	return points, len(points) >= 3
}

func readSVGPoint(tokens []string, index int) (svgPoint, int, bool) {
	if index+1 >= len(tokens) {
		return svgPoint{}, index, false
	}
	x, okX := parseSVGNumber(tokens[index])
	y, okY := parseSVGNumber(tokens[index+1])
	return svgPoint{x: x, y: y}, index + 2, okX && okY
}

func readSVGQuadratic(tokens []string, index int) (svgPoint, svgPoint, int, bool) {
	control, next, ok := readSVGPoint(tokens, index)
	if !ok {
		return svgPoint{}, svgPoint{}, index, false
	}
	end, next, ok := readSVGPoint(tokens, next)
	return control, end, next, ok
}

func readSVGCubic(tokens []string, index int) (svgPoint, svgPoint, svgPoint, int, bool) {
	c1, next, ok := readSVGPoint(tokens, index)
	if !ok {
		return svgPoint{}, svgPoint{}, svgPoint{}, index, false
	}
	c2, next, ok := readSVGPoint(tokens, next)
	if !ok {
		return svgPoint{}, svgPoint{}, svgPoint{}, index, false
	}
	end, next, ok := readSVGPoint(tokens, next)
	return c1, c2, end, next, ok
}

func readSVGNumberToken(tokens []string, index int) (float64, int, bool) {
	if index >= len(tokens) {
		return 0, index, false
	}
	value, ok := parseSVGNumber(tokens[index])
	return value, index + 1, ok
}

func appendQuadratic(points []svgPoint, p0 svgPoint, p1 svgPoint, p2 svgPoint) []svgPoint {
	for step := 1; step <= svgCurveSteps; step++ {
		t := float64(step) / svgCurveSteps
		mt := 1 - t
		points = append(points, svgPoint{
			x: mt*mt*p0.x + 2*mt*t*p1.x + t*t*p2.x,
			y: mt*mt*p0.y + 2*mt*t*p1.y + t*t*p2.y,
		})
	}
	return points
}

func appendCubic(points []svgPoint, p0 svgPoint, p1 svgPoint, p2 svgPoint, p3 svgPoint) []svgPoint {
	for step := 1; step <= svgCurveSteps; step++ {
		t := float64(step) / svgCurveSteps
		mt := 1 - t
		points = append(points, svgPoint{
			x: mt*mt*mt*p0.x + 3*mt*mt*t*p1.x + 3*mt*t*t*p2.x + t*t*t*p3.x,
			y: mt*mt*mt*p0.y + 3*mt*mt*t*p1.y + 3*mt*t*t*p2.y + t*t*t*p3.y,
		})
	}
	return points
}

func svgPathTokens(d string) []string {
	var tokens []string
	for i := 0; i < len(d); {
		ch := d[i]
		if ch == ',' || ch == ' ' || ch == '\n' || ch == '\t' || ch == '\r' {
			i++
			continue
		}
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			tokens = append(tokens, string(ch))
			i++
			continue
		}
		start := i
		i++
		for i < len(d) {
			ch = d[i]
			if ch == ',' || ch == ' ' || ch == '\n' || ch == '\t' || ch == '\r' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				break
			}
			if (ch == '-' || ch == '+') && i > start && d[i-1] != 'e' && d[i-1] != 'E' {
				break
			}
			i++
		}
		tokens = append(tokens, d[start:i])
	}
	return tokens
}

func isSVGPathCommand(token string) bool {
	return len(token) == 1 && strings.ContainsRune("MmLlHhVvQqCcZz", rune(token[0]))
}

func svgPointInPolygon(x float64, y float64, points []svgPoint) bool {
	inside := false
	j := len(points) - 1
	for i := range points {
		yi, yj := points[i].y, points[j].y
		xi, xj := points[i].x, points[j].x
		intersects := (yi > y) != (yj > y)
		if intersects && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}
