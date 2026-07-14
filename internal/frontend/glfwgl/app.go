//go:build glfw

package glfwgl

import (
	"image/color"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
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
	uiScale          float32
	contentScaleX    float32
	contentScaleY    float32
	status           statusState
	overlays         overlayRender
	scriptRT         *script.Runtime
	notice           string
	noticeUntil      time.Time
	suppressNextChar bool
	lastStats        time.Time
	lastTitle        string
	lastCwd          string
	lastBellCount    int
	blinkStart       time.Time
	pendingReplies   [][]byte
	showStats        bool
	statsSpec        script.Spec
	statsSpecOK      bool
	zoom             zoomBindings
	link             linkState
	hudLines         []string // cached HUD rows; see refreshHUDCache
	hudColors        []color.RGBA
	hudNotice        string
	hudShowStats     bool
	hudCols, hudRows int
	hudStatsAt       time.Time
	fps              float64
	fpsFrames        uint64
	fpsTime          time.Time
	skippedGlyph     []bool // reused per-row scratch buffer to avoid per-frame allocs
	ligaturesActive  bool   // font.ligatures enabled AND the active shaper can substitute

	rowHashes, prevHashes, prevPrevHashes []uint64
	// Cursor rows of the last two rendered frames. The cursor is drawn outside
	// the hash-based row damage, so it needs the same buffer-age-2 treatment as
	// content: with a double-buffered back buffer alternating between the N-1 and
	// N-2 images, clearing only the N-1 cursor row leaves a ghost on the older
	// buffer. Marking both prior cursor rows damaged repaints the stale cell.
	lastCursorRow, prevCursorRow int
	damage                       damageState

	// On-demand render state. Main-thread only; the PTY reader must not touch
	// needsRedraw (it wakes the loop with glfw.PostEmptyEvent instead).
	needsRedraw    bool
	lastBlinkPhase bool
	lastStatsDraw  time.Time
	// termEventsPending marks that the parser advanced outside drainIncoming
	// (no-PTY fallback), so the next loop iteration must still dispatch
	// title/bell events. Deferring to the loop instead of dispatching inline
	// keeps a handler's term:write from re-entering event dispatch.
	termEventsPending bool
	// Deferred resize/scroll event state, drained once per loop iteration by
	// fireLifecycleEvents. Marked (not fired) at the mutation site because
	// term:scroll / term:set_font_size run inside a handler; firing there would
	// re-enter Lua dispatch (traps 3 & 5).
	resizeEventPending, scrollEventPending bool
	resizeEventCols, resizeEventRows       int
	scrollEventOffset                      int

	// wakeReady gates the reader's PostEmptyEvent to the window between
	// glfw.Init succeeding and glfw.Terminate running: the reader starts before
	// GLFW is initialized and can outlive it, and posting outside that window
	// panics. A wake skipped around the transitions self-heals within the
	// loop's 500ms bounded wait.
	wakeReady atomic.Bool

	// Modal scrollback search state. All fields are main-thread only. While
	// searching is true, key and char callbacks route to the search bar and
	// nothing reaches the PTY (app_search.go). Match position is stored in the
	// global (physical-row) index space; draw() converts it to a viewport row.
	searching      bool
	searchQuery    []rune
	searchHasMatch bool
	searchMatchRow int // global row (scrollback+live index space)
	searchMatchCol int // start cell column of the match
	searchMatchLen int // match length in runes (highlight cell span, v1)
	searchViewRow  int // frame-local: match's viewport row, or -1 when off-screen

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
		uiScale:    1,
		blinkStart: time.Now(),
	}
	if spec, ok := parseStatsHotkey(cfg.Render.StatsHotkey); ok {
		app.statsSpec, app.statsSpecOK = spec, true
	}
	app.initZoomHotkeys()
	// Replies queue up and flush after Advance returns so the PTY write never
	// happens while a.mu is held (a blocked write must not stall the drain
	// loop mid-parse). All access is main-thread only.
	app.parser.Reply = func(b []byte) {
		app.pendingReplies = append(app.pendingReplies, b)
	}
	if cfg.Clipboard.OSC52 == "write" {
		app.parser.SetClipboard = func(s string) {
			if app.window != nil {
				app.window.SetClipboardString(s)
			}
		}
	}
	// The PTY spawns later, from runWindow, once the window + glyph atlas exist
	// and the real initial grid is known — see runWindow. This defer still
	// nil-checks because a.pty stays nil until then (and on spawn failure).
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
				a.wakeMainLoop()
			}
			if err != nil {
				if err != io.EOF {
					a.incoming <- []byte("\r\n[pty read error: " + err.Error() + "]\r\n")
					a.wakeMainLoop()
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
	// Registered after the Terminate defer so it runs first (LIFO): the reader
	// must stop posting wakes before GLFW tears down.
	a.wakeReady.Store(true)
	defer a.wakeReady.Store(false)

	glfw.WindowHint(glfw.ContextVersionMajor, 2)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.Resizable, glfw.True)
	w, err := glfw.CreateWindow(a.cfg.Window.Width, a.cfg.Window.Height, "CervTerm", nil, nil)
	if err != nil {
		return err
	}
	a.window = w
	if icons := windowIcons(); len(icons) > 0 {
		w.SetIcon(icons)
	}
	applyDarkTitleBar(w)
	w.MakeContextCurrent()
	swapInterval := 1
	if !a.cfg.Render.VSync {
		swapInterval = 0
	}
	glfw.SwapInterval(swapInterval)
	if err := gl.Init(); err != nil {
		return err
	}
	sx, sy := w.GetContentScale()
	a.applyScale(sx, sy)
	atlas, err := newGlyphAtlasWithSpec(fontglyph.Spec{Family: a.cfg.Font.Family, Size: a.cfg.Font.Size, DPI: effectiveDPI(sx, sy), TextRaster: a.cfg.Render.TextRaster}, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
	if err != nil {
		return err
	}
	a.atlas = atlas
	defer func() { a.atlas.close() }()
	// Probe ligature support once (not per frame): stays off with SimpleShaper.
	a.ligaturesActive = a.cfg.Font.Ligatures && atlas.supportsLigatures()
	a.cellW = float32(atlas.cellW)
	a.cellH = float32(atlas.cellH)
	a.installCallbacks()
	// Spawn the PTY now that cellW/cellH are final, sized to the real initial
	// grid so no startup resize repaints the shell and duplicates its banner.
	a.spawnInitialPTY(w)

	// Paint the first frame before any event arrives, and dispatch any term
	// events produced by pre-loop parser feeds (the no-PTY startup banner).
	a.needsRedraw = true
	a.termEventsPending = true

	return a.runLoop(w)
}

func (a *App) bracketedPasteMode() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.BracketedPasteMode()
}

func (a *App) installCallbacks() {
	a.window.SetContentScaleCallback(func(_ *glfw.Window, scaleX, scaleY float32) {
		a.rebuildForContentScale(scaleX, scaleY)
		a.requestRedraw()
	})
	// A framebuffer size change with the same cols/rows (padding remainder,
	// DPI move) still needs a repaint that resizeToWindow would miss.
	a.window.SetFramebufferSizeCallback(func(_ *glfw.Window, _, _ int) {
		a.requestRedraw()
	})
	a.window.SetCharCallback(func(_ *glfw.Window, char rune) {
		if a.suppressNextChar {
			a.suppressNextChar = false
			return
		}
		// While the search bar is open, printable input edits the query and never
		// reaches the PTY (trap 1). closeSearch restores this callback's normal
		// flow exactly by clearing a.searching.
		if a.searching {
			a.searchAppendRune(char)
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
		// Search owns the keyboard while open: the ctrl+shift+f chord toggles the
		// bar (fixed v1) and, while searching, every key routes to search handling
		// and nothing reaches the PTY (trap 1) — checked before script keys and
		// the stats toggle so those chords cannot leak through the bar.
		if a.handleSearchKey(key, mods) {
			return
		}
		if a.dispatchScriptKey(key, mods, action == glfw.Press) {
			return
		}
		if action == glfw.Press && a.statsSpecOK {
			if spec, ok := specFromGLFW(key, mods); ok && spec == a.statsSpec {
				a.showStats = !a.showStats
				a.requestRedraw()
				return
			}
		}
		// Built-in zoom and Shift+scroll bindings. Checked after script keys so a
		// user's Lua binding can still override, and before the PTY encode path so
		// the chords are consumed rather than sent to the shell.
		if a.handleZoomKey(key, mods) {
			return
		}
		if a.handleScrollKey(key, mods) {
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
			a.clearHover()
			a.requestRedraw()
			return
		}
		if action == glfw.Release {
			a.selectionEnd = point
			a.selecting = false
			// A plain click (no drag → selectionActive stays false) over a URL
			// opens it; a drag is a text selection and never opens a link.
			if !a.selectionActive && a.handleLinkClick(point) {
				a.requestRedraw()
				return
			}
			a.requestRedraw()
			return
		}
	})
	a.window.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		if a.sendMouseMove(x, y) {
			return
		}
		if !a.selecting {
			a.updateHover(x, y)
			return
		}
		a.selectionEnd = a.pointFromPixels(float32(x), float32(y))
		a.selectionActive = true
		a.requestRedraw()
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
		moved := a.term.ScrollViewport(rows)
		a.mu.Unlock()
		// A wheel tick at the clamp moves nothing: skip the redraw so no frame is
		// drawn (the event still woke the loop; nothing damages).
		if moved {
			a.requestRedraw()
			a.markScrollEvent()
		}
	})
	a.window.SetFocusCallback(func(_ *glfw.Window, focused bool) {
		// The script focus event is independent of the terminal's focus-report
		// mode. The callback runs on the loop thread (not inside a handler), so
		// firing inline cannot re-enter Lua dispatch.
		a.fireScriptEvent(func() error { return a.scriptRT.FireFocus(a, focused) })
		a.mu.Lock()
		enabled := a.term.FocusEventsMode()
		a.mu.Unlock()
		if !enabled {
			return
		}
		if focused {
			a.writeInput("\x1b[I")
		} else {
			a.writeInput("\x1b[O")
		}
	})
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
	text := a.Selection()
	if text == "" {
		return false
	}
	a.SetClipboard(text)
	return true
}

func (a *App) writeInputBytes(data []byte) {
	if a.pty != nil {
		_, _ = a.pty.Write(data)
		return
	}
	// No-PTY fallback: the parser is fed directly, so no PTY echo will come
	// back to wake the loop — request the repaint and event dispatch here.
	a.mu.Lock()
	a.parser.Advance(a.term, data)
	a.mu.Unlock()
	a.requestRedraw()
	a.termEventsPending = true
}

func (a *App) writeInput(s string) {
	if a.pty != nil {
		_, _ = a.pty.Write([]byte(s))
		return
	}
	a.mu.Lock()
	a.parser.Advance(a.term, []byte(s))
	a.mu.Unlock()
	a.requestRedraw()
	a.termEventsPending = true
}
