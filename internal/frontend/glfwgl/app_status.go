//go:build glfw

package glfwgl

import "math"

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
func (a *App) syncStatusSegments() { a.ensureScriptLifecycleController().syncStatus(a, a) }

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
	width := float32(len(runes))*a.cellW + 2*pad
	if width > float32(windowWidth) {
		width = float32(windowWidth)
	}
	first, last := coveredTerminalRows(a.drawOriginY, a.cellH, a.drawOriginY, a.cellH, a.snap.Rows)
	a.status.geometry = statusGeometry{visible: true, width: width, firstRow: first, lastRow: last}
}

func coveredTerminalRows(y, height, paddingY, cellH float32, rows int) (int, int) {
	first := int(math.Floor(float64((y - paddingY) / cellH)))
	last := int(math.Ceil(float64((y+height-paddingY)/cellH))) - 1
	first = max(0, first)
	last = min(rows-1, last)
	return first, last
}

func (a *App) drawStatusBand(windowWidth int, chrome chromeColors) {
	a.paint(statusBandLayout(a.status.display, a.status.geometry.width, windowWidth, a.drawOriginY, a.cellH, a.uiScale, chrome.background, chrome.accent))
}
