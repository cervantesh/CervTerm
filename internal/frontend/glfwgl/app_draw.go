//go:build glfw

package glfwgl

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/render"
	cervtermtheme "cervterm/internal/theme"
)

// hudCache holds the cached HUD rows and the inputs they were built from; see
// refreshHUDCache. Main-thread only.
type hudCache struct {
	lines     []string // cached HUD rows
	colors    []color.RGBA
	notice    string
	showStats bool
	cols      int
	rows      int
	statsAt   time.Time
}

func (a *App) draw() {
	// One timestamp and blink phase for the whole frame: sampling time.Now more
	// than once lets a blink boundary or notice expiry land between samples,
	// painting one state while recording another and losing the corrective
	// repaint.
	frameNow := time.Now()
	frameBlink := a.blinkPhaseAt(frameNow)
	if a.notice != "" && frameNow.After(a.noticeUntil) {
		a.notice = "" // expired: this frame paints without it and stops re-triggering
	}

	w, h := a.window.GetFramebufferSize()
	// Drive the renderer's Resize hook on a real size change (and on the first frame,
	// since lastFB* are seeded to -1) so a swapchain/drawable backend recreates before
	// the frame; BeginFrame then just re-establishes the coordinate space.
	if w != a.lastFBW || h != a.lastFBH {
		a.r.Resize(w, h)
		a.lastFBW, a.lastFBH = w, h
	}
	a.r.BeginFrame(w, h)

	palette := cervtermtheme.DefaultPalette()
	background := configColor(a.cfg.Colors.Background, themeColor(palette.Background))
	cursorColor := configColor(a.cfg.Colors.Cursor, themeColor(palette.Accent))
	selectionColor := configColor(a.cfg.Colors.SelectionBackground, color.RGBA{0x2A, 0x63, 0x77, 0xFF})
	defaultFG := configColor(a.cfg.Colors.Foreground, rgb(core.DefaultFG))
	a.updateFPS()

	// Title/cwd/bell events are fired in processTermEvents (once per data batch),
	// not here, so on-demand rendering does not delay them to the next repaint.
	a.mu.Lock()
	render.Capture(&a.snap, a.term)
	displayOffset := a.term.DisplayOffset()
	alternateScreen := a.term.AlternateScreenMode()
	// Convert the match's global (physical-row) index to a viewport row via the
	// canonical GlobalRowToViewport (same convention as CopyView, trap 2).
	// Off-screen matches yield -1. Done under the lock since it reads term state.
	a.search.viewRow = -1
	if a.search.hasMatch {
		if vr, ok := a.term.GlobalRowToViewport(a.search.matchRow); ok {
			a.search.viewRow = vr
		}
	}
	a.mu.Unlock()
	a.refreshLinks()
	a.prepareStatusBand(w)
	noticeVisible := a.notice != "" && frameNow.Before(a.noticeUntil)
	fullRedraw, damagedRows := a.prepareDamage(w, h, displayOffset, alternateScreen, noticeVisible, background)
	if fullRedraw {
		a.r.Clear(background)
	}

	var cursorRowOrder []int
	rowsDrawn := 0
	for r, damaged := range damagedRows {
		if !damaged {
			continue
		}
		rowsDrawn++
		if !fullRedraw {
			a.fillRect(0, a.paddingY+float32(r)*a.cellH, float32(w), a.cellH, background)
		}
		order := a.drawRow(r, background, selectionColor, defaultFG)
		if r == a.snap.CursorRow {
			cursorRowOrder = order
		}
	}

	if a.snap.CursorVisible {
		cursorRow, cursorCol := a.snap.CursorRow, a.snap.CursorCol
		if cursorRowOrder != nil {
			inverse := render.InversePermutation(cursorRowOrder)
			if cursorCol >= 0 && cursorCol < len(inverse) {
				cursorCol = inverse[cursorCol]
			}
		}
		x := a.paddingX + float32(cursorCol)*a.cellW
		y := a.paddingY + float32(cursorRow)*a.cellH
		a.drawCursor(x, y, cursorColor, frameBlink)
	}

	a.damage.rowsDrawn = rowsDrawn
	a.drawLinkUnderline(cursorColor)
	a.drawOverlays()
	a.drawHUD(w, h, palette, frameNow)
	a.drawStatusBand(w, palette)
	a.drawSearchBar(w, h, palette)
	a.recordDamageFrame(w, h, displayOffset, alternateScreen, noticeVisible, background, rowsDrawn)

	// Record exactly what this frame rendered so shouldRedraw detects the next
	// blink flip / stats-window elapse against the painted state.
	a.lastBlinkPhase = frameBlink
	if a.showStats {
		a.lastStatsDraw = frameNow
	}
}

// updateFPS derives a frames-per-second reading from the cumulative frame
// counter over a ~500ms window. Cheap enough to run every frame.
func (a *App) updateFPS() {
	now := time.Now()
	if a.fpsTime.IsZero() {
		a.fpsTime, a.fpsFrames = now, a.meter.Frames()
		return
	}
	if d := now.Sub(a.fpsTime); d >= 500*time.Millisecond {
		cur := a.meter.Frames()
		a.fps = float64(cur-a.fpsFrames) / d.Seconds()
		a.fpsFrames, a.fpsTime = cur, now
	}
}

// refreshHUDCache rebuilds the cached HUD lines only when an input that shapes
// them changes, so steady-state frames reuse the composed strings instead of
// re-running fmt.Sprintf every frame. Rebuild triggers: stats toggled on/off,
// the 500ms stats window elapsed, cols/rows changed, or the visible notice
// text differs from what was cached. The notice is independent of showStats, so
// all four (showStats, notice) combinations are handled.
func (a *App) refreshHUDCache(palette cervtermtheme.Palette, now time.Time) {
	noticeVisible := a.notice != "" && now.Before(a.noticeUntil)
	curNotice := ""
	if noticeVisible {
		curNotice = a.notice
	}
	// Stats numbers only need 500ms freshness (same cadence shouldRedraw uses).
	// A just-toggled-on panel or a cols/rows change forces an immediate rebuild
	// so no stale numbers flash before the next window rolls over.
	statsStale := a.showStats && (!a.hud.showStats ||
		a.cols != a.hud.cols || a.rows != a.hud.rows ||
		now.Sub(a.hud.statsAt) >= 500*time.Millisecond)
	if !statsStale && curNotice == a.hud.notice && a.showStats == a.hud.showStats {
		return
	}

	a.hud.lines = a.hud.lines[:0]
	a.hud.colors = a.hud.colors[:0]
	if a.showStats {
		s := a.meter.Snapshot()
		a.hud.lines = append(a.hud.lines,
			fmt.Sprintf("CervTerm  %dx%d  %.0f fps  rows:%d/%d  raster:%s  %s %.0f", a.cols, a.rows, a.fps, a.damage.rowsDrawn, a.snap.Rows, a.cfg.Render.TextRaster, a.cfg.Font.Family, a.cfg.Font.Size),
			fmt.Sprintf("%.1f KB read  heap %.1f MB  mallocs %d  GC %d  pause %s", float64(s.Bytes)/1024, float64(s.HeapAlloc)/(1024*1024), s.Allocs, s.NumGC, s.LastGCPause))
		a.hud.colors = append(a.hud.colors, themeColor(palette.Muted), themeColor(palette.Muted))
		a.hud.statsAt = now
	}
	if noticeVisible {
		a.hud.lines = append(a.hud.lines, a.notice)
		a.hud.colors = append(a.hud.colors, themeColor(palette.Accent))
	}
	a.hud.showStats = a.showStats
	a.hud.notice = curNotice
	a.hud.cols, a.hud.rows = a.cols, a.rows
}

// drawHUD overlays the optional two-row stats panel (toggled by the stats
// hotkey) and any transient notice on top of the terminal, so the terminal
// itself has no permanent chrome.
func (a *App) drawHUD(w, h int, palette cervtermtheme.Palette, now time.Time) {
	a.refreshHUDCache(palette, now)
	a.paint(hudLayout(a.hud.lines, a.hud.colors, a.cellW, a.cellH, a.uiScale, themeColor(palette.Accent)))
}

// paint executes a draw-list, translating each command into the corresponding
// renderer call. The renderer owns the translucent-blend state (BLEND stays
// enabled), which the alpha in chromeBoxColor relies on.
func (a *App) paint(cmds []drawCmd) {
	if len(cmds) == 0 {
		return
	}
	for _, c := range cmds {
		switch c.kind {
		case cmdRect:
			a.fillRect(c.x, c.y, c.w, c.h, c.col)
		case cmdText:
			a.drawString(c.text, c.x, c.y, c.col, 1)
		}
	}
}

// searchHighlightColor tints the current match cells under the glyphs, a warm
// amber distinct from the cool selection fill so the two never read as the same.
var searchHighlightColor = color.RGBA{0x7A, 0x5C, 0x12, 0xFF}

// drawSearchBar renders the modal search overlay at the bottom of the window,
// mirroring drawHUD's translucent-fill + accent-line + drawString style. It is
// drawn only while the bar is open; closing repaints a clean frame (the search
// state is in the damage global-fallback list).
func (a *App) drawSearchBar(w, h int, palette cervtermtheme.Palette) {
	a.paint(searchBarLayout(a.search.active, string(a.search.query), a.search.hasMatch, w, h, a.cellH, a.uiScale, themeColor(palette.Accent), themeColor(palette.Muted)))
}

func (a *App) drawTextDecorations(x, y, w, h float32, c color.RGBA, attr core.Attr) {
	if attr.Underline {
		a.fillRect(x, y+h-2, w, 1, c)
	}
	if attr.Strikethrough {
		a.fillRect(x, y+h*0.55, w, 1, c)
	}
}

func (a *App) drawString(s string, x, y float32, c color.RGBA, scale float32) {
	for _, r := range s {
		a.drawRune(r, x, y, c, scale, 0)
		x += a.cellW * scale
	}
}

func (a *App) drawRune(r rune, x, y float32, c color.RGBA, scale, skew float32) {
	a.atlas.drawRune(r, x, y, c, scale, skew)
}

func (a *App) drawCluster(cluster string, cellSpan int, x, y float32, c color.RGBA, scale, skew float32) bool {
	return a.atlas.drawCluster(cluster, cellSpan, x, y, c, scale, skew)
}

func (a *App) drawRunGlyph(run string, cellSpan int, x, y float32, c color.RGBA, scale, skew float32) bool {
	return a.atlas.drawRun(run, cellSpan, x, y, c, scale, skew)
}

// drawCursor renders the cursor using the frame's precomputed blink phase so
// the painted phase always matches the lastBlinkPhase recorded for this frame.
// The phase is time-based and independent of the config blink switch —
// DECSCUSR blink styles must animate even when the configured cursor is steady.
func (a *App) drawCursor(x, y float32, c color.RGBA, blinkPhase bool) {
	thickness := cursorThicknessPixels(a.cfg.Cursor.Thickness, a.cellW, a.cellH)
	shape, blink := a.cfg.Cursor.Shape, a.cfg.Cursor.Blink
	switch a.snap.CursorStyle.Shape() {
	case core.CursorShapeBlock:
		shape = "block"
	case core.CursorShapeUnderline:
		shape = "underline"
	case core.CursorShapeBar:
		shape = "beam"
	}
	if b, ok := a.snap.CursorStyle.Blink(); ok {
		blink = b
	}
	if blink && !blinkPhase {
		return
	}
	switch shape {
	case "block":
		a.fillRect(x, y, a.cellW, a.cellH, c)
	case "beam":
		a.fillRect(x, y, thickness, a.cellH, c)
	default:
		a.fillRect(x, y+a.cellH-thickness, a.cellW, thickness, c)
	}
}

func cursorThicknessPixels(configured float64, cellW, cellH float32) float32 {
	if configured <= 0 {
		configured = 0.15
	}
	base := cellH
	if configured <= 1 {
		base = cellH
		if cellW < cellH {
			base = cellW
		}
		px := float32(configured) * base
		if px < 1 {
			return 1
		}
		return px
	}
	return float32(configured)
}

func configColor(hex string, fallback color.RGBA) color.RGBA {
	if len(hex) != 7 || hex[0] != '#' {
		return fallback
	}
	r, errR := strconv.ParseUint(hex[1:3], 16, 8)
	g, errG := strconv.ParseUint(hex[3:5], 16, 8)
	b, errB := strconv.ParseUint(hex[5:7], 16, 8)
	if errR != nil || errG != nil || errB != nil {
		return fallback
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}

func (a *App) fillRect(x, y, w, h float32, c color.RGBA) {
	a.r.FillRect(x, y, w, h, c)
}

func themeColor(c cervtermtheme.Color) color.RGBA { return color.RGBA{c.R, c.G, c.B, 0xFF} }

func rgb(c core.RGB) color.RGBA { return color.RGBA{c.R, c.G, c.B, 0xFF} }
func brighten(c color.RGBA) color.RGBA {
	return color.RGBA{min(255, c.R+28), min(255, c.G+28), min(255, c.B+28), c.A}
}

func dim(c color.RGBA) color.RGBA {
	return color.RGBA{uint8(float32(c.R) * 0.55), uint8(float32(c.G) * 0.55), uint8(float32(c.B) * 0.55), c.A}
}
