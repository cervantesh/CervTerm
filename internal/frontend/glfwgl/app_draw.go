//go:build glfw

package glfwgl

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/render"
	termsel "cervterm/internal/selection"
	cervtermtheme "cervterm/internal/theme"

	"github.com/go-gl/gl/v2.1/gl"
)

func (a *App) draw() {
	w, h := a.window.GetFramebufferSize()
	gl.Viewport(0, 0, int32(w), int32(h))
	gl.MatrixMode(gl.PROJECTION)
	gl.LoadIdentity()
	gl.Ortho(0, float64(w), float64(h), 0, -1, 1)
	gl.MatrixMode(gl.MODELVIEW)
	gl.LoadIdentity()

	palette := cervtermtheme.DefaultPalette()
	background := configColor(a.cfg.Colors.Background, themeColor(palette.Background))
	cursorColor := configColor(a.cfg.Colors.Cursor, themeColor(palette.Accent))
	selectionColor := configColor(a.cfg.Colors.SelectionBackground, color.RGBA{0x2A, 0x63, 0x77, 0xFF})
	defaultFG := configColor(a.cfg.Colors.Foreground, rgb(core.DefaultFG))
	glClearColor(background)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.Disable(gl.TEXTURE_2D)
	a.updateFPS()

	a.mu.Lock()
	render.Capture(&a.snap, a.term)
	a.mu.Unlock()
	if a.snap.Title != a.lastTitle {
		a.lastTitle = a.snap.Title
		if a.snap.Title == "" {
			a.window.SetTitle("CervTerm")
		} else {
			a.window.SetTitle("CervTerm · " + a.snap.Title)
		}
		a.fireScriptEvent(func() error { return a.scriptRT.FireTitle(a, a.snap.Title) })
	}
	// BellCount is monotonic; fire once per bell so bursts are not collapsed.
	for a.lastBellCount < a.snap.BellCount {
		a.lastBellCount++
		a.fireScriptEvent(func() error { return a.scriptRT.FireBell(a) })
	}

	var cursorRowOrder []int
	for r := 0; r < a.snap.Rows; r++ {
		rowCells := a.snap.Cells[r*a.snap.Cols : (r+1)*a.snap.Cols]
		var order []int
		if a.cfg.Render.Bidi {
			order = render.VisualOrder(rowCells)
			if r == a.snap.CursorRow {
				cursorRowOrder = order
			}
		}
		skippedGlyph := make([]bool, a.snap.Cols)
		for visualCol := 0; visualCol < a.snap.Cols; visualCol++ {
			logicalCol := visualCol
			if order != nil {
				logicalCol = order[visualCol]
			}
			cell := rowCells[logicalCol]
			x := a.paddingX + float32(visualCol)*a.cellW
			y := a.paddingY + float32(r)*a.cellH
			if cell.Attr.BG != core.DefaultBG {
				fillRect(x, y, a.cellW, a.cellH, rgb(cell.Attr.BG))
			}
			if a.selectionActive && termsel.Contains(termsel.Range{Start: a.selectionStart, End: a.selectionEnd}, termsel.Point{Row: r, Col: logicalCol}) {
				fillRect(x, y, a.cellW, a.cellH, selectionColor)
			}
			fg := defaultFG
			if cell.Attr.FG != core.DefaultFG {

				fg = rgb(cell.Attr.FG)

			}
			bg := background
			if cell.Attr.BG != core.DefaultBG {
				bg = rgb(cell.Attr.BG)
			}
			if cell.Attr.Inverse {
				fg, bg = bg, fg
				fillRect(x, y, a.cellW, a.cellH, bg)
			}
			if skippedGlyph[logicalCol] || cell.Rune == ' ' || cell.Rune == 0 || cell.WideContinuation {
				continue
			}
			if cell.Attr.Bold {
				fg = brighten(fg)
			}
			if cell.Attr.Dim {
				fg = dim(fg)
			}
			if rects, ok := render.BoxGlyph(cell.Rune, a.cellW, a.cellH); ok {
				for _, rc := range rects {
					c := fg
					if rc.Alpha < 1 {
						c.A = uint8(float32(fg.A) * rc.Alpha)
					}
					fillRect(x+rc.X, y+rc.Y, rc.W, rc.H, c)
				}
				drawTextDecorations(x, y, a.cellW, a.cellH, fg, cell.Attr)
				continue
			}
			skew := float32(0)
			if cell.Attr.Italic {
				skew = 0.2 * a.cellH
			}
			if cluster, ok := collectRenderCluster(a.snap.Cells, a.snap.Cols, r, logicalCol); ok {
				if a.drawCluster(cluster.Text, cluster.CellSpan, x, y, fg, 1, skew) {
					if cell.Attr.Bold {
						a.drawCluster(cluster.Text, cluster.CellSpan, x+1, y, fg, 1, skew)
					}
					drawTextDecorations(x, y, a.cellW*float32(cluster.CellSpan), a.cellH, fg, cell.Attr)
					for i := 1; i < cluster.CellSpan && logicalCol+i < a.snap.Cols; i++ {
						skippedGlyph[logicalCol+i] = true
					}
					continue
				}
			}
			a.drawRune(cell.Rune, x, y, fg, 1, skew)
			if cell.Attr.Bold {
				a.drawRune(cell.Rune, x+1, y, fg, 1, skew)
			}
			for _, combining := range cell.Combining {
				a.drawRune(combining, x, y, fg, 1, skew)
				if cell.Attr.Bold {
					a.drawRune(combining, x+1, y, fg, 1, skew)
				}
			}
			drawTextDecorations(x, y, a.cellW, a.cellH, fg, cell.Attr)
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
		a.drawCursor(x, y, cursorColor)
	}

	a.drawHUD(w, h, palette)
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

// drawHUD overlays the optional two-row stats panel (toggled by the stats
// hotkey) and any transient notice on top of the terminal, so the terminal
// itself has no permanent chrome.
func (a *App) drawHUD(w, h int, palette cervtermtheme.Palette) {
	var lines []string
	var colors []color.RGBA
	if a.showStats {
		s := a.meter.Snapshot()
		lines = append(lines,
			fmt.Sprintf("CervTerm  %dx%d  %.0f fps  raster:%s  %s %.0f", a.cols, a.rows, a.fps, a.cfg.Render.TextRaster, a.cfg.Font.Family, a.cfg.Font.Size),
			fmt.Sprintf("%.1f KB read  heap %.1f MB  mallocs %d  GC %d  pause %s", float64(s.Bytes)/1024, float64(s.HeapAlloc)/(1024*1024), s.Allocs, s.NumGC, s.LastGCPause))
		colors = append(colors, themeColor(palette.Muted), themeColor(palette.Muted))
	}
	if time.Now().Before(a.noticeUntil) && a.notice != "" {
		lines = append(lines, a.notice)
		colors = append(colors, themeColor(palette.Accent))
	}
	if len(lines) == 0 {
		return
	}

	widest := 0
	for _, ln := range lines {
		if n := len([]rune(ln)); n > widest {
			widest = n
		}
	}
	pad := 6 * a.uiScale
	bx, by := pad, pad
	bw := float32(widest)*a.cellW + 2*pad
	bh := float32(len(lines))*a.cellH + 2*pad
	gl.Disable(gl.TEXTURE_2D)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	fillRect(bx, by, bw, bh, color.RGBA{0x10, 0x14, 0x1C, 0xF0})
	fillRect(bx, by, bw, max(1, a.uiScale), themeColor(palette.Accent))
	for i, ln := range lines {
		a.drawString(ln, bx+pad, by+pad+float32(i)*a.cellH, colors[i], 1)
	}
}

func drawTextDecorations(x, y, w, h float32, c color.RGBA, attr core.Attr) {
	if attr.Underline {
		fillRect(x, y+h-2, w, 1, c)
	}
	if attr.Strikethrough {
		fillRect(x, y+h*0.55, w, 1, c)
	}
}

func (a *App) drawString(s string, x, y float32, c color.RGBA, scale float32) {
	for _, r := range s {
		a.drawRune(r, x, y, c, scale, 0)
		x += a.cellW * scale
	}
}

func (a *App) drawRune(r rune, x, y float32, c color.RGBA, scale, skew float32) {
	gl.Enable(gl.TEXTURE_2D)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	glColor(c)
	a.atlas.drawRune(r, x, y, c, scale, skew)
	gl.Disable(gl.TEXTURE_2D)
}

func (a *App) drawCluster(cluster string, cellSpan int, x, y float32, c color.RGBA, scale, skew float32) bool {
	gl.Enable(gl.TEXTURE_2D)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	glColor(c)
	ok := a.atlas.drawCluster(cluster, cellSpan, x, y, c, scale, skew)
	gl.Disable(gl.TEXTURE_2D)
	return ok
}

// cursorBlinkPhase is the raw time-based blink phase, independent of whether
// the config enables blinking — DECSCUSR blink styles must animate even when
// the configured cursor is steady.
func (a *App) cursorBlinkPhase() bool {
	interval := a.cfg.Cursor.BlinkIntervalMS
	if interval <= 0 {
		interval = 1000
	}
	period := time.Duration(interval) * time.Millisecond
	return time.Since(a.blinkStart)%period < period/2
}

func (a *App) drawCursor(x, y float32, c color.RGBA) {
	thickness := cursorThicknessPixels(a.cfg.Cursor.Thickness, a.cellW, a.cellH)
	shape, blink := a.cfg.Cursor.Shape, a.cfg.Cursor.Blink
	switch a.snap.CursorStyle {
	case 1, 2:
		shape, blink = "block", a.snap.CursorStyle == 1
	case 3, 4:
		shape, blink = "underline", a.snap.CursorStyle == 3
	case 5, 6:
		shape, blink = "beam", a.snap.CursorStyle == 5
	}
	if blink && !a.cursorBlinkPhase() {
		return
	}
	switch shape {
	case "block":
		fillRect(x, y, a.cellW, a.cellH, c)
	case "beam":
		fillRect(x, y, thickness, a.cellH, c)
	default:
		fillRect(x, y+a.cellH-thickness, a.cellW, thickness, c)
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

func glClearColor(c color.RGBA) {
	gl.ClearColor(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, 1)
}
func glColor(c color.RGBA) {
	gl.Color4f(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
}

func fillRect(x, y, w, h float32, c color.RGBA) {
	glColor(c)
	gl.Begin(gl.QUADS)
	gl.Vertex2f(x, y)
	gl.Vertex2f(x+w, y)
	gl.Vertex2f(x+w, y+h)
	gl.Vertex2f(x, y+h)
	gl.End()
}

func themeColor(c cervtermtheme.Color) color.RGBA { return color.RGBA{c.R, c.G, c.B, 0xFF} }

func rgb(c core.RGB) color.RGBA { return color.RGBA{c.R, c.G, c.B, 0xFF} }
func brighten(c color.RGBA) color.RGBA {
	return color.RGBA{min(255, c.R+28), min(255, c.G+28), min(255, c.B+28), c.A}
}

func dim(c color.RGBA) color.RGBA {
	return color.RGBA{uint8(float32(c.R) * 0.55), uint8(float32(c.G) * 0.55), uint8(float32(c.B) * 0.55), c.A}
}
