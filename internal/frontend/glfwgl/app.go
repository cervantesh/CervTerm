//go:build glfw

package glfwgl

import (
	"errors"
	"runtime"
	"sync/atomic"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/fontglyph"
	"cervterm/internal/frontend/gpu"
	"cervterm/internal/input"
	"cervterm/internal/metrics"
	termmux "cervterm/internal/mux"
	"cervterm/internal/render"
	"cervterm/internal/script"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

type App struct {
	meter             metrics.Meter
	snap              render.Snapshot
	cfg               config.Config
	mux               *termmux.Mux
	focusedPane       termmux.PaneID
	paneUI            map[termmux.PaneID]*paneUIState
	pendingMuxEvents  []termmux.Event
	pendingPaneScroll map[termmux.PaneID]int
	pendingPaneResize map[termmux.PaneID]termmux.PaneGeometry

	window *glfw.Window
	r      gpu.Renderer
	atlas  *glyphAtlas

	// Last framebuffer size handed to the renderer; draw() calls r.Resize only when
	// it changes, so a backend recreates its swapchain/drawable on real size changes
	// (and once on the first frame — seeded to -1 in RunWithOptions) rather than
	// every frame.
	lastFBW, lastFBH int

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
	blinkStart       time.Time
	showStats        bool
	statsSpec        script.Spec
	statsSpecOK      bool
	zoom             zoomBindings
	link             linkState
	hud              hudCache
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

	// wakeReady gates the reader's PostEmptyEvent to the window between
	// glfw.Init succeeding and glfw.Terminate running: the reader starts before
	// GLFW is initialized and can outlive it, and posting outside that window
	// panics. A wake skipped around the transitions self-heals within the
	// loop's 500ms bounded wait.
	wakeReady atomic.Bool

	lterm            searchTerminal
	search           searchController
	selection        selectionState
	mouseReport      mouseReportState
	mouseCapturePane termmux.PaneID
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
		cfg:               cfg,
		scriptRT:          rt,
		cellW:             9,
		cellH:             16,
		uiScale:           1,
		blinkStart:        time.Now(),
		paneUI:            make(map[termmux.PaneID]*paneUIState),
		pendingPaneScroll: make(map[termmux.PaneID]int),
		pendingPaneResize: make(map[termmux.PaneID]termmux.PaneGeometry),
	}
	app.mux = termmux.New(nil, termmux.Options{
		Wake: app.wakeMainLoop,
		SetClipboard: func(_ termmux.PaneID, text string) {
			if app.window != nil && cfg.Clipboard.OSC52 == "write" {
				app.window.SetClipboardString(text)
			}
		},
	})
	if spec, ok := parseStatsHotkey(cfg.Render.StatsHotkey); ok {
		app.statsSpec, app.statsSpecOK = spec, true
	}
	app.initZoomHotkeys()
	return app.runWindow()
}

func (a *App) runWindow() error {
	if err := glfw.Init(); err != nil {
		return err
	}
	defer glfw.Terminate()
	// Stop reader goroutines while GLFW is still initialized. Clearing wakeReady
	// first prevents new wake attempts; Shutdown joins readers that may already
	// have observed true before glfw.Terminate runs.
	defer func() { _ = a.mux.Shutdown() }()
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
	// The GL context is current; build the renderer now so the atlas (which owns
	// the page geometry) can configure its textures in its own constructor.
	a.r = newGLRenderer(w)
	// -1 (not 0) so the first draw always drives Resize, even if the initial
	// framebuffer is 0x0 (0 is a valid size, so it cannot double as the sentinel).
	a.lastFBW, a.lastFBH = -1, -1
	sx, sy := w.GetContentScale()
	a.applyScale(sx, sy)
	atlas, err := newGlyphAtlasWithSpec(a.r, fontglyph.Spec{Family: a.cfg.Font.Family, Size: a.cfg.Font.Size, DPI: effectiveDPI(sx, sy), TextRaster: a.cfg.Render.TextRaster}, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
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

	return a.runLoop(w)
}

func (a *App) bracketedPasteMode() bool {
	_, view, ok := a.focusedView()
	return ok && view.BracketedPaste
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
		// flow exactly by clearing a.search.active.
		if a.search.active {
			a.search.appendRune(char)
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
		if a.search.handleKey(key, mods) {
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

		if a.handleMuxKey(key, mods) {
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
		x, y := a.window.GetCursorPos()
		if action == glfw.Press {
			if pane, _, ok := a.paneAtWindowPosition(x, y); ok {
				a.focusPane(pane)
			}
		}
		if a.sendMouseButton(button, action, mods) {
			return
		}
		if button != glfw.MouseButtonLeft {
			return
		}
		point := a.pointFromPixels(float32(x), float32(y))
		if action == glfw.Press {
			a.selection.dragging = true
			a.selection.active = false
			a.selection.start = point
			a.selection.end = point
			a.clearHover()
			a.requestRedraw()
			return
		}
		if action == glfw.Release {
			a.selection.end = point
			a.selection.dragging = false
			// A plain click (no drag → selectionActive stays false) over a URL
			// opens it; a drag is a text selection and never opens a link.
			if !a.selection.active && a.handleLinkClick(point) {
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
		if !a.selection.dragging {
			if pane, _, ok := a.paneAtWindowPosition(x, y); ok {
				a.updateHoverForPane(pane, x, y)
			} else {
				a.clearHover()
			}
			return
		}
		a.selection.end = a.pointFromPixels(float32(x), float32(y))
		a.selection.active = true
		a.requestRedraw()
	})
	a.window.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		// Ctrl+wheel zooms (font size), taking priority over app mouse reporting —
		// the standard terminal shortcut. GLFW does not pass modifiers to the
		// scroll callback, so query the live Ctrl state.
		if a.handleZoomWheel(yoff) {
			return
		}
		x, y := a.window.GetCursorPos()
		if pane, _, ok := a.paneAtWindowPosition(x, y); ok {
			a.focusPane(pane)
		}
		if a.sendMouseWheel(yoff, glfw.ModifierKey(0)) {
			return
		}
		rows := scrollRowsFromWheelDelta(yoff)
		if rows == 0 {
			return
		}
		moved, _ := a.mux.ScrollViewport(a.focusedPane, rows)
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
		a.fireScriptEvent(func() error { return a.scriptRT.FireFocus(a.hostForFocused(), focused) })
		_, view, ok := a.focusedView()
		enabled := ok && view.FocusEvents
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
	if a.focusedPane == 0 {
		return
	}
	events, err := a.mux.Write(a.focusedPane, data)
	if errors.Is(err, termmux.ErrPaneNotRunning) {
		if view, ok := a.mux.PaneView(a.focusedPane); ok && view.State == termmux.PaneStateFailed {
			events, err = a.mux.FeedFallback(a.focusedPane, data)
		}
	}
	if len(events) > 0 {
		a.pendingMuxEvents = append(a.pendingMuxEvents, events...)
	}
	if err != nil {
		a.Notify("input: " + err.Error())
	}
	if len(events) > 0 {
		a.requestRedraw()
	}
}

func (a *App) writeInput(s string) { a.writeInputBytes([]byte(s)) }
