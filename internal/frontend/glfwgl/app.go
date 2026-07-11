//go:build glfw

package glfwgl

import (
	"io"
	"math"
	"runtime"
	"sync"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/core"
	"cervterm/internal/fontglyph"
	"cervterm/internal/input"
	"cervterm/internal/metrics"
	ptyio "cervterm/internal/pty"
	"cervterm/internal/render"
	"cervterm/internal/script"
	termsel "cervterm/internal/selection"
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

	cols, rows       int
	cellW            float32
	cellH            float32
	paddingX         float32
	paddingY         float32
	status           string
	scriptRT         *script.Runtime
	notice           string
	noticeUntil      time.Time
	suppressNextChar bool
	lastStats        time.Time
	lastTitle        string
	blinkStart       time.Time

	selecting         bool
	selectionActive   bool
	selectionStart    termsel.Point
	selectionEnd      termsel.Point
	mouseReportDown   bool
	mouseReportButton input.MouseButton
	mouseReportMods   input.Mod
}

func Run() error {
	return RunWithConfig(config.Defaults())
}

func RunWithConfig(cfg config.Config) error {
	return RunWithOptions(cfg, nil)
}

func RunWithOptions(cfg config.Config, rt *script.Runtime) error {
	runtime.LockOSThread()
	app := &App{
		term:       core.NewTerminal(100, 32),
		cfg:        cfg,
		scriptRT:   rt,
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
	atlas, err := newGlyphAtlasWithSpec(fontglyph.Spec{Family: a.cfg.Font.Family, Size: a.cfg.Font.Size, DPI: 96})
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
		if a.suppressNextChar {
			a.suppressNextChar = false
			return
		}
		if encoded, ok := input.Encode(input.Event{Rune: char}); ok {
			a.writeInputBytes(encoded)
		}
	})
	a.window.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action != glfw.Press && action != glfw.Repeat {
			return
		}
		if a.dispatchScriptKey(key, mods, action == glfw.Press) {
			return
		}

		if a.handleClipboardKey(key, mods) {
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

func (a *App) dispatchScriptKey(key glfw.Key, mods glfw.ModifierKey, dispatch bool) bool {
	if a.scriptRT == nil {
		return false
	}
	spec, ok := specFromGLFW(key, mods)
	if !ok {
		return false
	}
	for i, binding := range a.scriptRT.Bindings() {
		if binding == spec {
			if dispatch {
				if err := a.scriptRT.Dispatch(i, a); err != nil {
					a.Notify("script error: " + err.Error())
				}
			}
			a.suppressNextChar = scriptKeyProducesChar(key, mods)
			return true
		}
	}
	return false
}

func scriptKeyProducesChar(key glfw.Key, mods glfw.ModifierKey) bool {
	if mods&(glfw.ModControl|glfw.ModAlt|glfw.ModSuper) != 0 {
		return false
	}
	if key >= glfw.KeyA && key <= glfw.KeyZ {
		return true
	}
	if key >= glfw.Key0 && key <= glfw.Key9 {
		return true
	}
	switch key {
	case glfw.KeySpace, glfw.KeyMinus, glfw.KeyEqual, glfw.KeyComma, glfw.KeyPeriod,
		glfw.KeySlash, glfw.KeyBackslash, glfw.KeySemicolon, glfw.KeyApostrophe,
		glfw.KeyGraveAccent:
		return true
	default:
		return false
	}
}

func (a *App) handleClipboardKey(key glfw.Key, mods glfw.ModifierKey) bool {
	if mods&glfw.ModControl != 0 && key == glfw.KeyV {
		text := a.window.GetClipboardString()
		a.writeInputBytes(input.EncodePaste(text, a.bracketedPasteMode()))
		return true
	}
	if mods&glfw.ModShift != 0 && key == glfw.KeyInsert {
		text := a.window.GetClipboardString()
		a.writeInputBytes(input.EncodePaste(text, a.bracketedPasteMode()))
		return true
	}
	if mods&glfw.ModControl != 0 && key == glfw.KeyInsert {
		_ = a.copySelectionToClipboard()
		return true
	}
	return false
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

func (a *App) WriteInput(s string) {
	a.writeInput(s)
}

func (a *App) Notify(msg string) {
	a.notice = msg
	a.noticeUntil = time.Now().Add(4 * time.Second)
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
