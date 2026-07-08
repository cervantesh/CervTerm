//go:build glfw

package glfwgl

import (
	"fmt"
	"image/color"
	"io"
	"math"
	"runtime"
	"sync"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/core"
	"cervterm/internal/input"
	"cervterm/internal/metrics"
	ptyio "cervterm/internal/pty"
	"cervterm/internal/render"
	termsel "cervterm/internal/selection"
	cervtermtheme "cervterm/internal/theme"
	"cervterm/internal/vt"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

type App struct {
	term   *core.Terminal
	parser vt.Parser
	meter  metrics.Meter
	pty    ptyio.Session
	snap   render.Snapshot
	cfg    config.Config

	mu       sync.Mutex
	incoming chan []byte
	window   *glfw.Window
	atlas    *glyphAtlas

	cols, rows int
	cellW      float32
	cellH      float32
	paddingX   float32
	paddingY   float32
	status     string
	lastStats  time.Time
	lastTitle  string
	blinkStart time.Time

	selecting         bool
	selectionActive   bool
	selectionStart    termsel.Point
	selectionEnd      termsel.Point
	mouseReportDown   bool
	mouseReportButton input.MouseButton
}

func Run() error {
	return RunWithConfig(config.Defaults())
}

func RunWithConfig(cfg config.Config) error {
	runtime.LockOSThread()
	app := &App{
		term:       core.NewTerminal(100, 32),
		cfg:        cfg,
		incoming:   make(chan []byte, 128),
		cellW:      9,
		cellH:      16,
		paddingX:   float32(cfg.Window.PaddingX),
		paddingY:   float32(cfg.Window.PaddingY),
		blinkStart: time.Now(),
	}
	if err := app.startPTY(); err != nil {
		app.parser.Advance(app.term, []byte("\x1b[96mCervTerm\x1b[0m\r\n\r\n"))
		app.parser.Advance(app.term, []byte("Local PTY unavailable on this platform/build.\r\n"))
		app.parser.Advance(app.term, []byte(err.Error()+"\r\n\r\n"))
		app.parser.Advance(app.term, []byte("Type to test the renderer and parser.\r\n"))
	}
	defer func() {
		if app.pty != nil {
			_ = app.pty.Close()
		}
	}()
	return app.runWindow()
}

func (a *App) startPTY() error {
	s, err := ptyio.NewLocalWithOptions(uint16(a.term.Rows()), uint16(a.term.Cols()), ptyio.Options{ShellProgram: a.cfg.Shell.Program, ShellArgs: a.cfg.Shell.Args, WorkingDirectory: a.cfg.Shell.WorkingDirectory, Env: a.cfg.Shell.Env})
	if err != nil {
		return err
	}
	a.pty = s
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := s.Reader().Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				a.incoming <- chunk
			}
			if err != nil {
				if err != io.EOF {
					a.incoming <- []byte("\r\n[pty read error: " + err.Error() + "]\r\n")
				}
				return
			}
		}
	}()
	return nil
}

func (a *App) runWindow() error {
	if err := glfw.Init(); err != nil {
		return err
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.ContextVersionMajor, 2)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.Resizable, glfw.True)
	w, err := glfw.CreateWindow(a.cfg.Window.Width, a.cfg.Window.Height, "CervTerm", nil, nil)
	if err != nil {
		return err
	}
	a.window = w
	w.MakeContextCurrent()
	glfw.SwapInterval(1)
	if err := gl.Init(); err != nil {
		return err
	}
	atlas, err := newGlyphAtlasWithSpec(fontSpec{Family: a.cfg.Font.Family, Size: a.cfg.Font.Size, DPI: 96})
	if err != nil {
		return err
	}
	a.atlas = atlas
	a.cellW = float32(atlas.cellW)
	a.cellH = float32(atlas.cellH)
	a.installCallbacks()

	for !w.ShouldClose() {
		glfw.PollEvents()
		a.drainIncoming()
		a.resizeToWindow()
		a.draw()
		w.SwapBuffers()
		a.meter.AddFrame()
	}
	return nil
}

func (a *App) bracketedPasteMode() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.BracketedPasteMode()
}

func (a *App) installCallbacks() {
	a.window.SetCharCallback(func(_ *glfw.Window, char rune) {
		if encoded, ok := input.Encode(input.Event{Rune: char}); ok {
			a.writeInputBytes(encoded)
		}
	})
	a.window.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action != glfw.Press && action != glfw.Repeat {
			return
		}

		event, hasEvent := inputEventFromGLFW(key, mods)
		if hasEvent {
			switch input.ClipboardShortcut(event) {
			case input.ClipboardCopy:
				_ = a.copySelectionToClipboard()
				return
			case input.ClipboardPaste:
				text := a.window.GetClipboardString()
				a.writeInputBytes(input.EncodePaste(text, a.bracketedPasteMode()))
				return
			}
		}

		if key == glfw.KeyC && mods&glfw.ModControl != 0 && a.copySelectionToClipboard() {
			return
		}

		if !hasEvent {
			return
		}
		if encoded, ok := input.EncodeWithMode(event, a.inputMode()); ok {
			a.writeInputBytes(encoded)
		}
	})
	a.window.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		if a.sendMouseButton(button, action, mods) {
			return
		}
		if button != glfw.MouseButtonLeft {
			return
		}
		x, y := a.window.GetCursorPos()
		point := a.pointFromPixels(float32(x), float32(y))
		if action == glfw.Press {
			a.selecting = true
			a.selectionActive = false
			a.selectionStart = point
			a.selectionEnd = point
			return
		}
		if action == glfw.Release {
			a.selectionEnd = point
			a.selecting = false
			return
		}
	})
	a.window.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		if a.sendMouseMove(x, y) {
			return
		}
		if !a.selecting {
			return
		}
		a.selectionEnd = a.pointFromPixels(float32(x), float32(y))
		a.selectionActive = true
	})
	a.window.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		if a.sendMouseWheel(yoff, glfw.ModifierKey(0)) {
			return
		}
		rows := scrollRowsFromWheelDelta(yoff)
		if rows == 0 {
			return
		}
		a.mu.Lock()
		a.term.ScrollViewport(rows)
		a.mu.Unlock()
	})
}

func (a *App) copySelectionToClipboard() bool {
	if !a.selectionActive {
		return false
	}
	a.mu.Lock()
	render.Capture(&a.snap, a.term)
	a.mu.Unlock()

	text := termsel.Text(a.snap, termsel.Range{Start: a.selectionStart, End: a.selectionEnd})
	if text == "" {
		return false
	}
	a.window.SetClipboardString(text)
	return true
}

func (a *App) pointFromPixels(x, y float32) termsel.Point {
	col := int((x - a.paddingX) / a.cellW)
	row := int((y - a.paddingY) / a.cellH)
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	if row >= a.rows {
		row = a.rows - 1
	}
	if col >= a.cols {
		col = a.cols - 1
	}
	return termsel.Point{Row: row, Col: col}
}

func scrollRowsFromWheelDelta(yoff float64) int {
	if yoff == 0 {
		return 0
	}
	rows := int(math.Round(yoff * 3))
	if rows == 0 {
		if yoff > 0 {
			return 1
		}
		return -1
	}
	return rows
}

func (a *App) writeInputBytes(data []byte) {
	if a.pty != nil {
		_, _ = a.pty.Write(data)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.parser.Advance(a.term, data)
}

func (a *App) writeInput(s string) {
	if a.pty != nil {
		_, _ = a.pty.Write([]byte(s))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.parser.Advance(a.term, []byte(s))
}

func (a *App) drainIncoming() {
	for {
		select {
		case data := <-a.incoming:
			a.mu.Lock()
			a.parser.Advance(a.term, data)
			a.mu.Unlock()
			a.meter.AddBytes(len(data))
		default:
			return
		}
	}
}

func (a *App) resizeToWindow() {
	w, h := a.window.GetFramebufferSize()
	cols := max(2, int((float32(w)-2*a.paddingX)/a.cellW))
	rows := max(1, int((float32(h)-a.paddingY-18)/a.cellH))
	if cols == a.cols && rows == a.rows {
		return
	}
	a.cols, a.rows = cols, rows
	a.mu.Lock()
	a.term.Resize(cols, rows)
	a.mu.Unlock()
	if a.pty != nil {
		_ = a.pty.Resize(ptyio.Size{Rows: uint16(rows), Cols: uint16(cols)})
	}
}

func (a *App) draw() {
	w, h := a.window.GetFramebufferSize()
	gl.Viewport(0, 0, int32(w), int32(h))
	gl.MatrixMode(gl.PROJECTION)
	gl.LoadIdentity()
	gl.Ortho(0, float64(w), float64(h), 0, -1, 1)
	gl.MatrixMode(gl.MODELVIEW)
	gl.LoadIdentity()

	palette := cervtermtheme.DefaultPalette()
	background := themeColor(palette.Background)
	panel := themeColor(palette.Surface)
	accent := themeColor(palette.Accent)
	glClearColor(background)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.Disable(gl.TEXTURE_2D)
	fillRect(10, 10, float32(w-20), float32(h-20), panel)
	fillRect(10, 10, float32(w-20), 28, themeColor(palette.Chrome))
	fillRect(22, 20, 9, 9, themeColor(palette.ANSI[1]))
	fillRect(38, 20, 9, 9, themeColor(palette.ANSI[3]))
	fillRect(54, 20, 9, 9, themeColor(palette.ANSI[2]))
	fillRect(0, 38, float32(w), 1, accent)

	if time.Since(a.lastStats) > 500*time.Millisecond {
		s := a.meter.Snapshot()
		a.status = fmt.Sprintf("CervTerm · %dx%d · %.1f KB read · heap %.1f MB · mallocs %d · GC %d · last pause %s",
			a.cols, a.rows, float64(s.Bytes)/1024, float64(s.HeapAlloc)/(1024*1024), s.Allocs, s.NumGC, s.LastGCPause)
		a.lastStats = time.Now()
	}
	a.drawString(a.status, 78, 16, themeColor(palette.Muted), 1)

	a.mu.Lock()
	render.Capture(&a.snap, a.term)
	a.mu.Unlock()
	if a.snap.Title != "" && a.snap.Title != a.lastTitle {
		a.lastTitle = a.snap.Title
		a.window.SetTitle("CervTerm · " + a.snap.Title)
	}

	for r := 0; r < a.snap.Rows; r++ {
		for c := 0; c < a.snap.Cols; c++ {
			cell := a.snap.Cells[r*a.snap.Cols+c]
			x := a.paddingX + float32(c)*a.cellW
			y := a.paddingY + float32(r)*a.cellH
			if cell.Attr.BG != core.DefaultBG {
				fillRect(x, y, a.cellW, a.cellH, rgb(cell.Attr.BG))
			}
			if a.selectionActive && termsel.Contains(termsel.Range{Start: a.selectionStart, End: a.selectionEnd}, termsel.Point{Row: r, Col: c}) {
				fillRect(x, y, a.cellW, a.cellH, color.RGBA{0x2A, 0x63, 0x77, 0xFF})
			}
			if cell.Rune != ' ' && cell.Rune != 0 && !cell.WideContinuation {
				fg := rgb(cell.Attr.FG)
				if cell.Attr.Bold {
					fg = brighten(fg)
				}
				a.drawRune(cell.Rune, x, y, fg, 1)
			}
		}
	}

	if a.snap.CursorVisible && math.Mod(time.Since(a.blinkStart).Seconds(), 1.0) < 0.55 {
		cursorRow, cursorCol := a.snap.CursorRow, a.snap.CursorCol
		x := a.paddingX + float32(cursorCol)*a.cellW
		y := a.paddingY + float32(cursorRow)*a.cellH
		fillRect(x, y+a.cellH-3, a.cellW, 2, accent)
	}
}

func (a *App) drawString(s string, x, y float32, c color.RGBA, scale float32) {
	for _, r := range s {
		a.drawRune(r, x, y, c, scale)
		x += a.cellW * scale
	}
}

func (a *App) drawRune(r rune, x, y float32, c color.RGBA, scale float32) {
	gl.Enable(gl.TEXTURE_2D)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	glColor(c)
	a.atlas.drawRune(r, x, y, scale)
	gl.Disable(gl.TEXTURE_2D)
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
