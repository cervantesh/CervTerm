//go:build glfw

package glfwgl

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/frontend/gpu"
	termmux "cervterm/internal/mux"
	"cervterm/internal/render"
)

// hudCache holds the cached HUD rows and the inputs they were built from; see
// refreshHUDCache. Main-thread only.
type hudCache struct {
	lines     []string // cached HUD rows
	colors    []color.RGBA
	notice    string
	showStats bool
	cols      int
	chrome    chromeColors
	rows      int
	statsAt   time.Time
}

func (a *App) draw() {
	frameNow := time.Now()
	frameBlink := a.blinkPhaseAt(frameNow)
	if a.notice != "" && frameNow.After(a.noticeUntil) {
		a.notice = ""
	}
	w, h := a.window.GetFramebufferSize()
	if w != a.lastFBW || h != a.lastFBH {
		a.r.Resize(w, h)
		a.lastFBW, a.lastFBH = w, h
	}
	a.applyPreparedBackgroundResize()
	a.requestBackgroundResize(w, h)
	a.r.BeginFrame(w, h)

	a.chrome = resolveChromeColors(a.cfg)
	backgroundBase := configColor(a.cfg.Colors.Background, color.RGBA{0x08, 0x0B, 0x12, 0xFF})
	background := applyOpacity(backgroundBase, a.cfg.Window.BackgroundOpacity)
	cursorColor := configColor(a.cfg.Colors.Cursor, a.chrome.accent)
	selectionColor := configColor(a.cfg.Colors.SelectionBackground, color.RGBA{0x2A, 0x63, 0x77, 0xFF})
	paletteBase := configuredPaletteBase(a.cfg.Colors)
	a.updateFPS()
	a.restoreBackgroundSurface(background, w, h)

	layout, err := a.mux.Layout()
	if err != nil {
		a.Notify("layout: " + err.Error())
		return
	}
	focused, _ := a.mux.FocusedPane()
	baseOriginX, baseOriginY := a.drawOriginX, a.drawOriginY
	a.saveActivePaneUI()
	rowsDrawn := 0
	for _, geometry := range layout.Panes {
		view, ok := a.mux.PaneView(geometry.Pane)
		if !ok {
			continue
		}
		state := a.ensurePaneUI(geometry.Pane)
		a.activatePaneFont(geometry.Pane)
		a.snap = view.Snapshot
		panePalette := a.snap.PaletteOverrides.Apply(paletteBase)
		colorResolver := panePalette.ColorResolver()
		defaultFG := rgb(panePalette.FG)
		paneBackgroundBase := panePaletteBackground(backgroundBase, panePalette, a.snap.PaletteOverrides)
		paneBackground := effectivePaneBackground(backgroundBase, paneBackgroundBase, a.snap.PaletteOverrides.BGSet, a.cfg.Window.BackgroundOpacity)
		a.selection, a.search, a.link, a.mouseReport = state.selection, state.search, state.link, state.mouseReport
		a.search.init(muxSearchTerminal{mux: a.mux, pane: geometry.Pane}, a.requestRedraw)
		a.search.viewRow = -1
		if a.search.hasMatch {
			if row, ok := a.mux.GlobalRowToViewport(geometry.Pane, a.search.matchRow); ok {
				a.search.viewRow = row
			}
		}
		a.drawOriginX = float32(geometry.Pixels.X)
		a.drawOriginY = float32(geometry.Pixels.Y)
		a.refreshLinks()
		a.r.PushClip(gpu.ClipRect{X: geometry.Pixels.X, Y: geometry.Pixels.Y, Width: geometry.Pixels.Width, Height: geometry.Pixels.Height})
		if paneNeedsFlatBackground(len(a.cfg.Background.Layers), a.snap.PaletteOverrides.BGSet) {
			a.replaceRect(float32(geometry.Pixels.X), float32(geometry.Pixels.Y), float32(geometry.Pixels.Width), float32(geometry.Pixels.Height), paneBackground)
		}
		var cursorRowOrder []int
		for row := 0; row < a.snap.Rows; row++ {
			rowsDrawn++
			order := a.drawRow(row, paneBackgroundBase, selectionColor, defaultFG, &colorResolver)
			if row == a.snap.CursorRow {
				cursorRowOrder = order
			}
		}
		if geometry.Pane == focused && a.snap.CursorVisible {
			cursorRow, cursorCol := a.snap.CursorRow, a.snap.CursorCol
			if cursorRowOrder != nil {
				inverse := render.InversePermutation(cursorRowOrder)
				if cursorCol >= 0 && cursorCol < len(inverse) {
					cursorCol = inverse[cursorCol]
				}
			}
			x := a.drawOriginX + float32(cursorCol)*a.cellW
			y := a.drawOriginY + float32(cursorRow)*a.cellH
			a.drawCursor(x, y, cursorColor, frameBlink)
		}
		a.drawLinkUnderline(cursorColor)
		if geometry.Pane == focused {
			a.drawOverlays()
		}
		a.r.PopClip()
		state.selection, state.search, state.link, state.mouseReport = a.selection, a.search, a.link, a.mouseReport
	}
	a.drawOriginX, a.drawOriginY = baseOriginX, baseOriginY
	for _, divider := range layout.Dividers {
		r := divider.Pixels
		a.fillRect(float32(r.X), float32(r.Y), float32(r.Width), float32(r.Height), a.chrome.split)
	}
	if view, ok := a.mux.PaneView(focused); ok {
		r := view.Geometry.Pixels
		if r.Width > 0 && r.Height > 0 {
			accent := a.chrome.accent
			if view.State == termmux.PaneStateExited || view.State == termmux.PaneStateFailed {
				accent = a.chrome.error
			}
			a.fillRect(float32(r.X), float32(r.Y), float32(r.Width), 1, accent)
			a.fillRect(float32(r.X), float32(r.Bottom()-1), float32(r.Width), 1, accent)
			a.fillRect(float32(r.X), float32(r.Y), 1, float32(r.Height), accent)
			a.fillRect(float32(r.Right()-1), float32(r.Y), 1, float32(r.Height), accent)
		}
	}
	if focused != 0 {
		a.focusedPane = focused
		a.loadPaneUI(focused)
		if view, ok := a.mux.PaneView(focused); ok {
			a.snap = view.Snapshot
			a.cols, a.rows = view.Snapshot.Cols, view.Snapshot.Rows
		}
	}
	a.retainVisibleFontContexts(layout)
	a.damage.rowsDrawn = rowsDrawn
	a.prepareStatusBand(w)
	a.drawHUD(w, h, a.chrome, frameNow)
	a.drawStatusBand(w, a.chrome)
	a.drawSearchBar(w, h, a.chrome)
	a.drawModal(w, h, a.chrome)
	a.drawScrollbar(frameNow, background, w, h)
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
func (a *App) refreshHUDCache(chrome chromeColors, now time.Time) {
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
	if !statsStale && curNotice == a.hud.notice && a.showStats == a.hud.showStats && chrome == a.hud.chrome {
		return
	}

	a.hud.lines = a.hud.lines[:0]
	a.hud.colors = a.hud.colors[:0]
	if a.showStats {
		s := a.meter.Snapshot()
		a.hud.lines = append(a.hud.lines,
			fmt.Sprintf("CervTerm  %dx%d  %.0f fps  rows:%d/%d  raster:%s  %s %.0f", a.cols, a.rows, a.fps, a.damage.rowsDrawn, a.snap.Rows, a.effectiveTextRaster(), a.cfg.Font.Family, a.FontSize()),
			fmt.Sprintf("%.1f KB read  heap %.1f MB  mallocs %d  GC %d  pause %s", float64(s.Bytes)/1024, float64(s.HeapAlloc)/(1024*1024), s.Allocs, s.NumGC, s.LastGCPause))
		a.hud.colors = append(a.hud.colors, chrome.muted, chrome.muted)
		a.hud.statsAt = now
	}
	if noticeVisible {
		a.hud.lines = append(a.hud.lines, a.notice)
		a.hud.colors = append(a.hud.colors, chrome.accent)
	}
	a.hud.showStats = a.showStats
	a.hud.notice = curNotice
	a.hud.cols, a.hud.rows = a.cols, a.rows
	a.hud.chrome = chrome
}

// drawHUD overlays the optional two-row stats panel (toggled by the stats
// hotkey) and any transient notice on top of the terminal, so the terminal
// itself has no permanent chrome.
func (a *App) drawHUD(w, h int, chrome chromeColors, now time.Time) {
	a.refreshHUDCache(chrome, now)
	a.paint(hudLayout(a.hud.lines, a.hud.colors, a.cellW, a.cellH, a.uiScale, chrome.background, chrome.accent))
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

// drawSearchBar renders the modal search overlay at the bottom of the window,
// mirroring drawHUD's translucent-fill + accent-line + drawString style. It is
// drawn only while the bar is open; closing repaints a clean frame (the search
// state is in the damage global-fallback list).
func (a *App) drawSearchBar(w, h int, chrome chromeColors) {
	a.paint(searchBarLayout(a.search.active, string(a.search.query), a.search.hasMatch, w, h, a.cellH, a.uiScale, chrome.background, chrome.accent, chrome.muted))
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
	if (len(hex) != 7 && len(hex) != 9) || hex[0] != '#' {
		return fallback
	}
	r, errR := strconv.ParseUint(hex[1:3], 16, 8)
	g, errG := strconv.ParseUint(hex[3:5], 16, 8)
	b, errB := strconv.ParseUint(hex[5:7], 16, 8)
	if errR != nil || errG != nil || errB != nil {
		return fallback
	}
	a := uint64(255)
	if len(hex) == 9 {
		var err error
		a, err = strconv.ParseUint(hex[7:9], 16, 8)
		if err != nil {
			return fallback
		}
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
}

func (a *App) replaceRect(x, y, w, h float32, c color.RGBA) {
	a.r.ReplaceRect(x, y, w, h, c)
}

func (a *App) fillRect(x, y, w, h float32, c color.RGBA) {
	a.r.FillRect(x, y, w, h, c)
}

func rgb(c core.RGB) color.RGBA { return color.RGBA{c.R, c.G, c.B, 0xFF} }
func brighten(c color.RGBA) color.RGBA {
	return color.RGBA{min(255, c.R+28), min(255, c.G+28), min(255, c.B+28), c.A}
}

func dim(c color.RGBA) color.RGBA {
	return color.RGBA{uint8(float32(c.R) * 0.55), uint8(float32(c.G) * 0.55), uint8(float32(c.B) * 0.55), c.A}
}
