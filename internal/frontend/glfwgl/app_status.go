//go:build glfw

package glfwgl

import (
	"image/color"
	"log"
	"math"
	"strings"

	cervtermtheme "cervterm/internal/theme"

	"github.com/go-gl/gl/v2.1/gl"
)

type statusGeometry struct {
	visible  bool
	width    float32
	firstRow int
	lastRow  int
}

type statusState struct {
	line     string
	display  string
	seq      int
	geometry statusGeometry
}

// syncStatusSegments rebuilds the composed status string only after a real
// script-store mutation. It runs after all script handlers for the loop pass so
// timer/event updates request a prompt on-demand repaint.
func (a *App) syncStatusSegments() {
	if a.scriptRT == nil {
		return
	}
	seq := a.scriptRT.StatusSeq()
	if seq == a.status.seq {
		return
	}
	a.status.seq = seq
	a.status.line = strings.Join(a.scriptRT.StatusSegments(), " · ")
	a.requestRedraw()
}

// prepareStatusBand derives the window-dependent display text and geometry
// before damage selection. The untruncated composed line remains cached by seq.
func (a *App) prepareStatusBand(windowWidth int) {
	a.status.display = ""
	a.status.geometry = statusGeometry{}
	if a.status.line == "" || windowWidth <= 0 || a.cellW <= 0 {
		return
	}
	pad := 6 * a.uiScale
	maxRunes := int((float32(windowWidth) - 2*pad) / a.cellW)
	if maxRunes < 1 {
		return
	}
	runes := []rune(a.status.line)
	if len(runes) > maxRunes {
		if maxRunes == 1 {
			runes = []rune{'…'}
		} else {
			runes = append([]rune{'…'}, runes[len(runes)-(maxRunes-1):]...)
		}
	}
	a.status.display = string(runes)
	log.Printf("DBG prepare: windowWidth=%d appCellW=%.3f atlasCellW=%d uiScale=%.4f pad=%.3f maxRunes=%d displayRunes=%d", windowWidth, a.cellW, a.atlas.cellW, a.uiScale, pad, maxRunes, len(runes))
	width := float32(len(runes))*a.cellW + 2*pad
	if width > float32(windowWidth) {
		width = float32(windowWidth)
	}
	first, last := coveredTerminalRows(a.paddingY, a.cellH, a.paddingY, a.cellH, a.snap.Rows)
	a.status.geometry = statusGeometry{visible: true, width: width, firstRow: first, lastRow: last}
}

func coveredTerminalRows(y, height, paddingY, cellH float32, rows int) (int, int) {
	first := int(math.Floor(float64((y - paddingY) / cellH)))
	last := int(math.Ceil(float64((y+height-paddingY)/cellH))) - 1
	first = max(0, first)
	last = min(rows-1, last)
	return first, last
}

func (a *App) drawStatusBand(windowWidth int, palette cervtermtheme.Palette) {
	g := a.status.geometry
	if !g.visible {
		return
	}
	pad := 6 * a.uiScale
	bx, by := float32(windowWidth)-g.width, a.paddingY
	bh := a.cellH
	fbw, _ := a.window.GetFramebufferSize()
	winw, _ := a.window.GetSize()
	sampleW := float32(a.atlas.cellW)
	if e, ok := a.atlas.cachedRune('S'); ok {
		sampleW = float32(a.atlas.cellW * max(1, e.cellSpan))
		log.Printf("DBG draw: windowWidth=%d fbw=%d winw=%d appCellW=%.3f atlasCellW=%d gWidth=%.2f bx=%.2f textStart=%.2f textEnd=%.2f sampleQuadW=%.2f cellSpanS=%d", windowWidth, fbw, winw, a.cellW, a.atlas.cellW, g.width, bx, bx+pad, bx+pad+float32(len([]rune(a.status.display)))*a.cellW, sampleW, e.cellSpan)
	}
	_ = sampleW
	gl.Disable(gl.TEXTURE_2D)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	fillRect(bx, by, g.width, bh, color.RGBA{0x10, 0x14, 0x1C, 0xF0})
	fillRect(bx, by, g.width, max(1, a.uiScale), themeColor(palette.Accent))
	a.drawString(a.status.display, bx+pad, by, themeColor(palette.Accent), 1)
}
