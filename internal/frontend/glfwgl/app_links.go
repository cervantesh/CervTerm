//go:build glfw

package glfwgl

import (
	"image/color"
	"os/exec"
	"runtime"

	termsel "cervterm/internal/selection"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// linkState holds the detected URLs for the current frame plus the hovered link
// and a lazily-created hand cursor. Detection runs once per drawn frame so idle
// terminals cost nothing; hover hit-tests against this cache on mouse move.
type linkState struct {
	links       []linkRegion
	hover       linkRegion
	hoverActive bool
	handCursor  *glfw.Cursor
}

// refreshLinks re-detects URLs from the freshly captured snapshot. Called from
// draw() on the main thread. If the hovered link no longer exists (content
// scrolled or changed), hover is dropped and the cursor restored.
func (a *App) refreshLinks() {
	a.link.links = detectLinks(a.snap.Cells, a.snap.Cols, a.snap.Rows)
	if a.link.hoverActive {
		if _, ok := linkAt(a.link.links, termsel.Point{Row: a.link.hover.row, Col: a.link.hover.startCol}); !ok {
			a.link.hoverActive = false
			if a.window != nil {
				a.window.SetCursor(nil)
			}
		}
	}
}

// updateHover recomputes the hovered link from a mouse position and, when it
// changes, swaps the cursor to a pointing hand and requests a repaint so the
// underline appears/moves. No-op when nothing changed.
func (a *App) updateHover(x, y float64) {
	if len(a.snap.Cells) == 0 {
		a.clearHover()
		return
	}
	p := a.pointFromPixels(float32(x), float32(y))
	l, ok := linkAt(a.link.links, p)
	if ok == a.link.hoverActive && l == a.link.hover {
		return
	}
	a.link.hover, a.link.hoverActive = l, ok
	if a.window != nil {
		if ok {
			a.window.SetCursor(a.ensureHandCursor())
		} else {
			a.window.SetCursor(nil)
		}
	}
	a.requestRedraw()
}

// clearHover drops any hover state (used when a drag-selection begins).
func (a *App) clearHover() {
	if !a.link.hoverActive {
		return
	}
	a.link.hoverActive = false
	if a.window != nil {
		a.window.SetCursor(nil)
	}
	a.requestRedraw()
}

func (a *App) ensureHandCursor() *glfw.Cursor {
	if a.link.handCursor == nil {
		a.link.handCursor = glfw.CreateStandardCursor(glfw.HandCursor)
	}
	return a.link.handCursor
}

// handleLinkClick opens the URL under a plain (non-drag) left click. Returns
// true when a link was opened so the caller can skip other click handling.
func (a *App) handleLinkClick(p termsel.Point) bool {
	l, ok := linkAt(a.link.links, p)
	if !ok {
		return false
	}
	if err := openURL(l.url); err != nil {
		a.Notify("no se pudo abrir el link: " + err.Error())
	}
	return true
}

// drawLinkUnderline underlines the hovered link's cells. Called in draw() after
// the terminal rows so it sits over the glyphs, before the system UI.
func (a *App) drawLinkUnderline(c color.RGBA) {
	if !a.link.hoverActive {
		return
	}
	l := a.link.hover
	if l.row < 0 || l.row >= a.snap.Rows {
		return
	}
	thickness := max(1, a.uiScale)
	x := a.drawOriginX + float32(l.startCol)*a.cellW
	y := a.drawOriginY + float32(l.row)*a.cellH + a.cellH - thickness
	w := float32(l.endCol-l.startCol+1) * a.cellW
	a.fillRect(x, y, w, thickness, c)
}

// openURL launches the OS default handler for a URL. On Windows this goes
// through url.dll's FileProtocolHandler, which avoids the shell-quoting pitfalls
// of "cmd /c start".
func openURL(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
