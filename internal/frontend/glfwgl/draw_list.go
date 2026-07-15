package glfwgl

import "image/color"

// chromeBoxColor is the shared translucent backdrop for all chrome surfaces
// (HUD, search bar, status band). Alpha 0xF0 requires BLEND in the executor.
var chromeBoxColor = color.RGBA{0x10, 0x14, 0x1C, 0xF0}

type drawCmdKind uint8

const (
	cmdRect drawCmdKind = iota // filled quad: x,y,w,h + color (alpha respected)
	cmdText                    // string at x,y + color; scale always 1
)

type drawCmd struct {
	kind drawCmdKind
	x, y float32
	w, h float32 // cmdRect only
	text string  // cmdText only
	col  color.RGBA
}

// hudLayout composes the optional stats panel and transient notice into a
// draw-list: a translucent box, an accent top line, and one text command per
// row. Pure layout — no GL. The box width uses the widest row measured in
// runes, not bytes, so multibyte glyphs are sized correctly.
func hudLayout(lines []string, colors []color.RGBA, cellW, cellH, uiScale float32, accent color.RGBA) []drawCmd {
	if len(lines) == 0 {
		return nil
	}
	widest := 0
	for _, ln := range lines {
		if n := len([]rune(ln)); n > widest {
			widest = n
		}
	}
	pad := 6 * uiScale
	bx, by := pad, pad
	bw := float32(widest)*cellW + 2*pad
	bh := float32(len(lines))*cellH + 2*pad
	cmds := make([]drawCmd, 0, len(lines)+2)
	cmds = append(cmds,
		drawCmd{kind: cmdRect, x: bx, y: by, w: bw, h: bh, col: chromeBoxColor},
		drawCmd{kind: cmdRect, x: bx, y: by, w: bw, h: max(1, uiScale), col: accent},
	)
	for i, ln := range lines {
		cmds = append(cmds, drawCmd{kind: cmdText, x: bx + pad, y: by + pad + float32(i)*cellH, text: ln, col: colors[i]})
	}
	return cmds
}

// searchBarLayout composes the modal search overlay pinned to the bottom of the
// window. Pure layout — no GL. Returns nil when the bar is inactive. The text
// color is muted only when the query is non-empty and has no match.
func searchBarLayout(active bool, query string, hasMatch bool, winW, winH int, cellH, uiScale float32, accent, muted color.RGBA) []drawCmd {
	if !active {
		return nil
	}
	line := "buscar: " + query
	switch {
	case len(query) == 0:
		// no suffix
	case hasMatch:
		line += "  [enter: siguiente]"
	default:
		line += "  sin resultados"
	}
	pad := 6 * uiScale
	bh := cellH + 2*pad
	by := float32(winH) - bh
	textColor := accent
	if len(query) > 0 && !hasMatch {
		textColor = muted
	}
	return []drawCmd{
		{kind: cmdRect, x: 0, y: by, w: float32(winW), h: bh, col: chromeBoxColor},
		{kind: cmdRect, x: 0, y: by, w: float32(winW), h: max(1, uiScale), col: accent},
		{kind: cmdText, x: pad, y: by + pad, text: line, col: textColor},
	}
}

// statusBandLayout composes the right-aligned status band. Pure layout — no GL.
// Returns nil when there is nothing to display. The text baseline is by WITHOUT
// the pad offset, preserving the existing vertical asymmetry.
func statusBandLayout(display string, bandWidth float32, winW int, paddingY, cellH, uiScale float32, accent color.RGBA) []drawCmd {
	if display == "" {
		return nil
	}
	pad := 6 * uiScale
	bx := float32(winW) - bandWidth
	by := paddingY
	return []drawCmd{
		{kind: cmdRect, x: bx, y: by, w: bandWidth, h: cellH, col: chromeBoxColor},
		{kind: cmdRect, x: bx, y: by, w: bandWidth, h: max(1, uiScale), col: accent},
		{kind: cmdText, x: bx + pad, y: by, text: display, col: accent},
	}
}
