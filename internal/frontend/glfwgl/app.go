//go:build glfw

package glfwgl

import (
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/frontend/gpu"
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
	restoreAppearance      *projectionAppearance
	projectionBaseConfig   *config.Config
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
	startupConfigCommitted bool
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
	windowID                  termmux.WindowID
	controller                *windowController
	r                         gpu.Renderer
	backgroundSurface         gpu.BackgroundSurface
	backgroundSurfaceWidth    int
	backgroundSurfaceHeight   int
	backgroundRequestedWidth  int
	backgroundRequestedHeight int
	backgroundGeneration      uint64
	backgroundResizeResults   chan backgroundResizeResult
	atlas                     *glyphAtlas

	lastFBW, lastFBH int

	cols, rows                       int
	cellW                            float32
	cellH                            float32
	insets                           FramebufferInsets
	drawOriginX                      float32
	drawOriginY                      float32
	uiScale                          float32
	tabBar                           tabBarLayout
	tabBarPressed                    tabHit
	tabBarFirst                      int
	tabBarHeight                     int
	tabClose                         tabCloseConfirmation
	tabActivity                      map[termmux.TabID]bool
	contentScaleX                    float32
	contentScaleY                    float32
	status                           statusState
	overlays                         overlayRender
	scriptRT                         *script.Runtime
	scriptBundle                     *script.CandidateBundle
	scriptActivation                 *script.CandidateActivation
	legacyTransition                 *config.LegacyTealTransition
	scriptGeneration                 uint64
	commandPalette                   map[string]commandPaletteActivation
	workspaceSwitcher                map[string]workspaceSwitcherActivation
	quickSelect                      quickSelectActivation
	notice                           string
	noticeUntil                      time.Time
	charSuppression                  charSuppression
	textTarget                       committedTextTargetState
	composition                      compositionCoordinator
	candidateGeometry                candidateGeometryPublisher
	imeActivation                    projectionIMEActivation
	accessibilityActivation          projectionAccessibilityActivation
	accessibilityRuntime             projectionAccessibilityRuntime
	accessibilityCompositionRevision uint64
	lastStats                        time.Time
	blinkStart                       time.Time
	showStats                        bool
	statsSpec                        script.Spec
	statsSpecOK                      bool
	zoom                             zoomBindings
	actionBindings                   []keyActionBinding
	keyTable                         keyTableState
	link                             linkState
	linkLauncher                     urlLauncher
	clipboardSetter                  func(string)
	bellState
	notificationState
	hud             hudCache
	fps             float64
	fpsFrames       uint64
	fpsTime         time.Time
	skippedGlyph    []bool // reused per-row scratch buffer to avoid per-frame allocs
	ligaturesActive bool   // font.ligatures enabled AND the active shaper can substitute

	rowHashes, prevHashes, prevPrevHashes []uint64
	lastCursorRow, prevCursorRow          int
	damage                                damageState

	needsRedraw    bool
	presentation   presentationGate
	lastBlinkPhase bool
	lastStatsDraw  time.Time

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
		bellState:              bellState{bellDelivered: make(map[termmux.PaneID]int)},
	}
	app.initCompositionCoordinator()
	app.linkLauncher = platformURLLauncher{}
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
	restorePlan, restoreFound, restoreLoadErr := loadConfiguredRestorePlan(a.cfg)
	if restoreLoadErr != nil {
		log.Printf("layout restore ignored: %v", restoreLoadErr)
		restoreFound = false
	}
	if err := glfw.Init(); err != nil {
		return err
	}
	defer glfw.Terminate()
	defer a.shutdownProcessServices()
	a.wakeReady.Store(true)
	defer a.wakeReady.Store(false)
	defer a.discardConfigReloadWorkers()

	glfw.WindowHint(glfw.ContextVersionMajor, 2)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.Resizable, glfw.True)
	a.applyNativeWindowCreationHints()
	if restoreFound {
		monitors, monitorErr := currentGLFWMonitors()
		if monitorErr != nil {
			log.Printf("layout restore ignored: %v", monitorErr)
		} else if blueprint, prepareErr := prepareConfiguredRestore(a.cfg, restorePlan, monitors); prepareErr != nil {
			log.Printf("layout restore ignored: %v", prepareErr)
		} else if handled, restoreErr := a.tryRunRestoredWindow(blueprint); handled {
			return restoreErr
		} else if restoreErr != nil {
			log.Printf("layout restore failed; starting a fresh window: %v", restoreErr)
		}
	}
	w, err := glfw.CreateWindow(a.cfg.Window.Width, a.cfg.Window.Height, "CervTerm", nil, nil)
	if err != nil {
		return err
	}
	a.window = w
	if err := a.attachInitialWindowController(w); err != nil {
		return err
	}
	defer a.closeInitialWindowController()
	projectionAdopted := false
	defer func() {
		if !projectionAdopted {
			a.closeUnadoptedProjectionResources()
		}
	}()
	a.transparentFramebuffer = w.GetAttrib(glfw.TransparentFramebuffer) == glfw.True
	a.blurProvider = newBlurProvider(w)
	a.configureNativeWindow(w)
	a.applyWindowAppearance()
	if err := a.controller.activate(initialWindowID); err != nil {
		return err
	}
	swapInterval := 1
	if !a.cfg.Render.VSync {
		swapInterval = 0
	}
	glfw.SwapInterval(swapInterval)
	if err := gl.Init(); err != nil {
		return err
	}
	a.r = newGLRenderer(w)
	if err := a.prepareInitialBackgroundSurface(); err != nil {
		return err
	}
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
	a.ligaturesActive = atlas.supportsLigatures(a.cfg.Font.Ligatures)
	a.cellW = float32(atlas.cellW)
	a.cellH = float32(atlas.cellH)
	if err := a.applyInitialGridWindowPlan(w, sx, sy); err != nil {
		return err
	}
	a.initMux()
	if err := a.commitStartupConfiguration(); err != nil {
		return err
	}
	a.syncProcessServices()
	a.installCallbacks()
	a.spawnInitialPTY(w)

	if err := a.adoptInitialProjection(w); err != nil {
		return err
	}
	projectionAdopted = true
	a.needsRedraw = true

	return a.runLoop(w)
}

func (a *App) installCallbacks() {
	a.window.SetContentScaleCallback(func(_ *glfw.Window, scaleX, scaleY float32) {
		a.invalidateCandidateGeometry()
		a.rebuildForContentScale(scaleX, scaleY)
		a.requestAccessibilityRedraw()
	})
	a.window.SetFramebufferSizeCallback(func(_ *glfw.Window, _, _ int) {
		a.invalidateCandidateGeometry()
		a.requestAccessibilityRedraw()
	})
	a.window.SetSizeCallback(func(_ *glfw.Window, _, _ int) {
		a.invalidateCandidateGeometry()
		a.requestAccessibilityRedraw()
	})
	a.installAccessibilityWindowCallbacks()
	a.window.SetCharCallback(func(_ *glfw.Window, char rune) {
		a.routeGLFWChar(char)
	})
	a.window.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		a.handleKeyEvent(key, action, mods)
	})

	a.window.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		if a.handleModalMouseButton(button, action, mods) {
			return
		}
		x, y := a.window.GetCursorPos()
		if a.handleTabBarButton(button, action, x, y) {
			return
		}
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
			a.captureLinkPress(point)
			a.selection.dragging = true
			a.selection.active = false
			a.selection.start = point
			a.selection.end = point
			a.clearHover()
			a.requestAccessibilityRedraw()
			return
		}
		if action == glfw.Release {
			a.selection.end = point
			a.selection.dragging = false
			if !a.selection.active && a.handleLinkClick(point) {
				a.requestRedraw()
				return
			}
			a.requestAccessibilityRedraw()
		}
	})
	a.window.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		if a.handleModalCursorPos(x, y) {
			return
		}
		if a.pointerOverTabBar(x, y) {
			a.clearDividerCursor()
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
		a.requestAccessibilityRedraw()
	})
	a.window.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		if a.handleModalScroll(xoff, yoff) {
			return
		}
		x, y := a.window.GetCursorPos()
		if a.pointerOverTabBar(x, y) {
			return
		}
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
			a.requestAccessibilityRedraw()
			a.markScrollEvent()
		}
	})
	a.window.SetFocusCallback(func(_ *glfw.Window, focused bool) {
		a.recordNativeFocus(focused)
		if !focused {
			a.compositionNativeFocusChanged(false)
			a.keyTable.cancel()
			a.finishDividerDrag()
			a.clearDividerCursor()
			a.cancelMouseCapture()
			a.mouseBindingCapture = mouseBindingCapture{}
		}
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
