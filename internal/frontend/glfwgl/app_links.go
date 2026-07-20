//go:build glfw

package glfwgl

import (
	"image/color"
	"os/exec"
	"runtime"

	"cervterm/internal/linkpolicy"
	termmux "cervterm/internal/mux"
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
	press       linkPress
}

// refreshLinks re-detects URLs from the freshly captured snapshot. Called from
// draw() on the main thread. If the hovered link no longer exists (content
// scrolled or changed), hover is dropped and the cursor restored.
func (a *App) refreshLinks() {
	a.link.links = detectSnapshotLinks(a.snap.Cells, a.snap.Hyperlinks, a.snap.Cols, a.snap.Rows)
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

type linkPress struct {
	active bool
	pane   termmux.PaneID
	target linkRegion
}

func (a *App) captureLinkPress(p termsel.Point) {
	a.link.press = linkPress{}
	cached, ok := linkAt(a.link.links, p)
	if !ok {
		return
	}
	fresh, ok := a.freshLinkAt(p)
	if !ok || fresh != cached {
		return
	}
	a.link.press = linkPress{active: true, pane: a.focusedPane, target: fresh}
}

// handleLinkClick is the only hyperlink activation entry point. The caller invokes it
// for a plain pointer release; this method re-resolves the current pane snapshot before
// policy evaluation so terminal output cannot activate or race a stale hovered target.
func (a *App) handleLinkClick(p termsel.Point) bool {
	press := a.link.press
	a.link.press = linkPress{}
	if !press.active {
		return false
	}
	fresh, ok := a.freshLinkAt(p)
	if !ok || press.pane != a.focusedPane || fresh != press.target {
		a.Notify("enlace cambiado; inténtalo de nuevo")
		a.clearHover()
		return true
	}
	decision := linkpolicy.Evaluate(fresh.url, linkpolicy.Activation{Explicit: true, Fresh: true})
	if !decision.Allowed() {
		a.Notify("enlace bloqueado (" + decision.SafeLabel + "): " + string(decision.Denial))
		return true
	}
	if a.linkLauncher == nil || a.linkLauncher.Launch(decision.URI) != nil {
		a.Notify("no se pudo abrir " + decision.SafeLabel)
	}
	return true
}

func (a *App) freshLinkAt(p termsel.Point) (linkRegion, bool) {
	if a.mux == nil || a.focusedPane == 0 {
		return linkRegion{}, false
	}
	view, ok := a.mux.PaneView(a.focusedPane)
	if !ok {
		return linkRegion{}, false
	}
	links := detectSnapshotLinks(view.Snapshot.Cells, view.Snapshot.Hyperlinks, view.Snapshot.Cols, view.Snapshot.Rows)
	return linkAt(links, p)
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

type urlLauncher interface{ Launch(string) error }

type platformURLLauncher struct{}

// Launch runs only after the centralized policy accepted a fresh explicit user gesture.
// It must remain on the GLFW-owned OS thread.
func (platformURLLauncher) Launch(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
