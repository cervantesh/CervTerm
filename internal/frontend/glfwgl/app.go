//go:build glfw

package glfwgl

import (
	"fmt"
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/frontend/gpu"
	"cervterm/internal/input"
	"cervterm/internal/metrics"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"cervterm/internal/render"
	"cervterm/internal/script"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

type App struct {
	meter                  metrics.Meter
	snap                   render.Snapshot
	cfg                    config.Config
	chrome                 chromeColors
	desiredCfg             config.Config
	composedCfg            config.Config
	safeFonts              bool
	composedProvenance     []config.ProvenanceRecord
	runtimeScopes          config.RuntimeScopes
	configScope            config.ConfigScopeID
	runtimeOverrideRecords []config.RuntimeOverrideRecord
	configStateInitialized bool
	pendingConfig          []config.ConfigChange
	lastConfigReloadError  string
	lastReloadNoticeError  string
	configReloadAsync      configReloadAsyncState
	configPath             string
	candidateOptions       script.CandidateOptions
	configWatch            configWatchState
	configWatchHashes      map[string][32]byte
	reloadPending          bool
	tealPublicationOptions config.TealPublicationOptions
	mux                    *termmux.Mux
	focusedPane            termmux.PaneID
	paneUI                 map[termmux.PaneID]*paneUIState
	pendingMuxEvents       []termmux.Event
	pendingPaneScroll      map[termmux.PaneID]int
	pendingPaneResize      map[termmux.PaneID]termmux.PaneGeometry

	transparentFramebuffer bool
	transparencyWarned     bool
	blurProvider           BlurProvider
	blurProviderName       string
	blurStatus             BlurStatus
	blurWarned             bool
	blurWarnedStatus       BlurStatus

	window                    *glfw.Window
	r                         gpu.Renderer
	backgroundSurface         gpu.BackgroundSurface
	backgroundSurfaceWidth    int
	backgroundSurfaceHeight   int
	backgroundRequestedWidth  int
	backgroundRequestedHeight int
	backgroundGeneration      uint64
	backgroundResizeResults   chan backgroundResizeResult
	atlas                     *glyphAtlas

	// Last framebuffer size sent to r.Resize; seeded to -1 so the first frame
	// initializes the backend and later frames resize only on real changes.
	lastFBW, lastFBH int

	cols, rows       int
	cellW            float32
	cellH            float32
	insets           FramebufferInsets
	drawOriginX      float32
	drawOriginY      float32
	uiScale          float32
	contentScaleX    float32
	contentScaleY    float32
	status           statusState
	overlays         overlayRender
	scriptRT         *script.Runtime
	scriptBundle     *script.CandidateBundle
	scriptActivation *script.CandidateActivation
	legacyTransition *config.LegacyTealTransition
	scriptGeneration uint64
	commandPalette   map[string]commandPaletteActivation
	quickSelect      quickSelectActivation
	notice           string
	noticeUntil      time.Time
	suppressNextChar bool
	lastStats        time.Time
	blinkStart       time.Time
	showStats        bool
	statsSpec        script.Spec
	statsSpecOK      bool
	zoom             zoomBindings
	actionBindings   []keyActionBinding
	keyTable         keyTableState
	link             linkState
	hud              hudCache
	fps              float64
	fpsFrames        uint64
	fpsTime          time.Time
	skippedGlyph     []bool // reused per-row scratch buffer to avoid per-frame allocs
	ligaturesActive  bool   // font.ligatures enabled AND the active shaper can substitute

	rowHashes, prevHashes, prevPrevHashes []uint64
	// Cursor rows need buffer-age-2 damage because the cursor bypasses row hashes;
	// repaint both prior rows so alternating back buffers cannot retain a ghost.
	lastCursorRow, prevCursorRow int
	damage                       damageState

	// On-demand render state. Main-thread only; the PTY reader must not touch
	// needsRedraw (it wakes the loop with glfw.PostEmptyEvent instead).
	needsRedraw    bool
	presentation   presentationGate
	lastBlinkPhase bool
	lastStatsDraw  time.Time

	// wakeReady prevents PostEmptyEvent before GLFW init or after termination;
	// a skipped transition wake self-heals within the bounded loop wait.
	wakeReady atomic.Bool

	lterm               searchTerminal
	search              searchController
	modal               modal.Coordinator
	selection           selectionState
	mouseReport         mouseReportState
	mouseCapturePane    termmux.PaneID
	mouseBindingCapture mouseBindingCapture
	mouseClicks         mouseClickState
	divider             dividerInteraction
	scrollbar           scrollbarState
}

func runWithSource(cfg config.Config, rt *script.Runtime, bundle *script.CandidateBundle, activation *script.CandidateActivation, legacyTransition *config.LegacyTealTransition, watchPaths []string, watchHashes map[string][32]byte, sourcePath string, options script.CandidateOptions) error {
	runtime.LockOSThread()
	var initialProvenance []config.ProvenanceRecord
	initialOptions := options.Clone()
	if bundle != nil {
		initialProvenance = bundle.Provenance()
		initialOptions = bundle.Options()
	}
	authoredCfg := cfg.Clone()
	safeFonts := safeFontsMode.Swap(false)
	activeCfg := effectiveStartupConfig(authoredCfg, safeFonts)
	app := &App{
		cfg:                    activeCfg,
		desiredCfg:             authoredCfg.Clone(),
		composedCfg:            authoredCfg.Clone(),
		safeFonts:              safeFonts,
		configStateInitialized: true,
		composedProvenance:     initialProvenance,
		configPath:             sourcePath,
		candidateOptions:       initialOptions,
		scriptRT:               rt,
		scriptGeneration:       1,
		scriptBundle:           bundle,
		scriptActivation:       activation,
		legacyTransition:       legacyTransition,
		configWatchHashes:      watchHashes,
		cellW:                  9,
		cellH:                  16,
		uiScale:                1,
		blinkStart:             time.Now(),
		paneUI:                 make(map[termmux.PaneID]*paneUIState),
		pendingPaneScroll:      make(map[termmux.PaneID]int),
		pendingPaneResize:      make(map[termmux.PaneID]termmux.PaneGeometry),
	}
	app.configScope = app.runtimeScopes.NewScope()
	app.configWatch = newConfigWatchState(watchPaths...)
	if spec, ok := parseStatsHotkey(cfg.Render.StatsHotkey); ok {
		app.statsSpec, app.statsSpecOK = spec, true
	}
	app.initZoomHotkeys()
	if activation == nil {
		app.initActionBindings()
	}
	defer app.runtimeScopes.CloseScope(app.configScope)
	defer func() {
		if app.legacyTransition != nil {
			if err := app.legacyTransition.Rollback(); err != nil {
				log.Printf("rollback legacy Teal transition: %v", err)
			}
			app.legacyTransition = nil
		}
	}()
	defer func() {
		if app.scriptBundle != nil {
			app.scriptBundle.Close()
			app.scriptBundle = nil
			app.scriptRT = nil
		} else if app.scriptRT != nil {
			app.scriptRT.Close()
			app.scriptRT = nil
		}
	}()
	return app.runWindow()
}

func (a *App) runWindow() error {
	if err := glfw.Init(); err != nil {
		return err
	}
	defer glfw.Terminate()
	// Stop reader goroutines while GLFW is still initialized. The mux is created
	// only after the prepared atlas is adopted, so every early return is nil-safe.
	defer func() {
		if a.mux != nil {
			_ = a.mux.Shutdown()
		}
	}()
	// Stop reader wake posts before GLFW teardown (registered after Terminate for LIFO).
	a.wakeReady.Store(true)
	defer a.wakeReady.Store(false)
	defer a.discardConfigReloadWorkers()

	glfw.WindowHint(glfw.ContextVersionMajor, 2)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.Resizable, glfw.True)
	a.applyNativeWindowCreationHints()
	w, err := glfw.CreateWindow(a.cfg.Window.Width, a.cfg.Window.Height, "CervTerm", nil, nil)
	if err != nil {
		return err
	}
	a.window = w
	defer a.closeDividerCursors()
	a.transparentFramebuffer = w.GetAttrib(glfw.TransparentFramebuffer) == glfw.True
	a.blurProvider = newBlurProvider(w)
	defer func() {
		if err := a.blurProvider.Close(); err != nil {
			log.Printf("close blur provider %q: %v", a.blurProvider.Name(), err)
		}
	}()
	a.configureNativeWindow(w)
	a.applyWindowAppearance()
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
	rendererAdopted := false
	defer func() {
		if !rendererAdopted && a.r != nil {
			a.r.Destroy()
		}
	}()
	if err := a.prepareInitialBackgroundSurface(); err != nil {
		return err
	}
	defer a.closeBackgroundSurface()
	// -1 (not 0) so the first draw always drives Resize, even if the initial
	// framebuffer is 0x0 (0 is a valid size, so it cannot double as the sentinel).
	a.lastFBW, a.lastFBH = -1, -1
	sx, sy := w.GetContentScale()
	a.applyScale(sx, sy)
	stages := defaultFontInstallationStages()
	plan, err := newStartupFontInstallationPlan(a.cfg, effectiveDPI(sx, sy), a.effectiveTextRaster(), a.safeFonts)
	if err != nil {
		return err
	}
	prepared, err := prepareFontInstallation(plan, stages)
	if err != nil {
		return err
	}
	defer prepared.Close()
	atlas, err := prepared.adopt(a.r, stages)
	if err != nil {
		return err
	}
	a.atlas = atlas
	rendererAdopted = true
	defer func() {
		if a.atlas != nil {
			a.closeBackgroundSurface()
			a.atlas.close()
		}
	}()
	// Probe ligature support once (not per frame): stays off with SimpleShaper.
	a.ligaturesActive = atlas.supportsLigatures(a.cfg.Font.Ligatures)
	a.cellW = float32(atlas.cellW)
	a.cellH = float32(atlas.cellH)
	if err := a.applyInitialGridWindowPlan(w, sx, sy); err != nil {
		return err
	}
	a.initMux()
	// Every fallible window/renderer/font resource now exists. Publish staged v2
	// Teal immediately before the remaining in-memory activation and PTY spawn.
	if watchHashesChanged(a.configWatchHashes) {
		return fmt.Errorf("configuration sources changed during frontend preparation; reload the newest generation")
	}
	if a.scriptBundle != nil {
		if _, err := a.scriptBundle.PublishTeal(a.tealPublicationOptions); err != nil {
			return err
		}
	}
	if a.scriptActivation != nil {
		a.scriptRT = a.scriptActivation.Commit()
		a.scriptActivation = nil
		a.initActionBindings()
	}
	if a.legacyTransition != nil {
		a.legacyTransition.Commit()
		a.legacyTransition = nil
	}
	a.installCallbacks()
	// Spawn the PTY now that cellW/cellH are final, sized to the real initial
	// grid so no startup resize repaints the shell and duplicates its banner.
	a.spawnInitialPTY(w)

	// Paint the first frame before any event arrives, and dispatch any term
	// events produced by pre-loop parser feeds (the no-PTY startup banner).
	a.needsRedraw = true

	return a.runLoop(w)
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
		if a.handleModalChar(char) {
			return
		}
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
		a.handleKeyEvent(key, action, mods)
	})

	a.window.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		if a.handleModalMouseButton(button, action, mods) {
			return
		}
		x, y := a.window.GetCursorPos()
		if a.handleConfiguredMouseButton(button, action, mods, x, y) {
			return
		}
		if a.divider.active {
			if button == glfw.MouseButtonLeft && action == glfw.Release {
				a.finishDividerDrag()
				a.updateDividerCursor(x, y)
			}
			return
		}
		if button == glfw.MouseButtonLeft && action == glfw.Press && a.mouseCapturePane == 0 && a.beginDividerDrag(x, y) {
			return
		}
		fx, fy := a.windowToFramebuffer(x, y)
		if a.handleScrollbarButton(button, action, fx, fy) {
			a.clearDividerCursor()
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
			if !a.selection.active && a.handleLinkClick(point) {
				a.requestRedraw()
				return
			}
			a.requestRedraw()
		}
	})
	a.window.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		if a.handleModalCursorPos(x, y) {
			return
		}
		if a.mouseCapturePane != 0 {
			a.clearDividerCursor()
			a.sendMouseMove(x, y)
			return
		}
		if a.handleConfiguredMouseDrag(x, y) {
			return
		}
		if a.dragDivider(x, y) {
			return
		}
		fx, fy := a.windowToFramebuffer(x, y)
		if a.handleScrollbarMove(fx, fy) {
			a.clearDividerCursor()
			return
		}
		reported := a.sendMouseMove(x, y)
		if a.updateDividerCursor(x, y) {
			return
		}
		if reported {
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
		if a.handleModalScroll(xoff, yoff) {
			return
		}
		x, y := a.window.GetCursorPos()
		if a.handleConfiguredMouseWheel(yoff, x, y) {
			return
		}
		if a.handleZoomWheel(yoff) {
			return
		}
		fx, fy := a.windowToFramebuffer(x, y)
		if a.handleScrollbarWheel(yoff, fx, fy) {
			return
		}
		rows := scrollRowsFromWheelDelta(yoff, a.cfg.Scrolling.WheelMultiplier)
		if rows == 0 {
			return
		}
		moved, _ := a.mux.ScrollViewport(a.focusedPane, rows)
		if moved {
			a.scrollbar.lastActivity = time.Now()
			a.requestRedraw()
			a.markScrollEvent()
		}
	})
	a.window.SetFocusCallback(func(_ *glfw.Window, focused bool) {
		if !focused {
			a.keyTable.cancel()
			a.finishDividerDrag()
			a.clearDividerCursor()
			a.cancelMouseCapture()
			a.mouseBindingCapture = mouseBindingCapture{}
		}
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
