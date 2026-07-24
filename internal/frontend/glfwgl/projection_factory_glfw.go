//go:build glfw

package glfwgl

import (
	"fmt"
	"time"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	ptyio "cervterm/internal/pty"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// glfwProjectionFactory is the production transaction which constructs a
// complete native projection before the mux publishes its WindowID. Process
// configuration, scripting and mux services remain shared; every field which
// can accumulate UI, renderer, atlas, background, blur, callback, damage or
// presentation state belongs to the child App.
type glfwProjectionFactory struct {
	owner *App
}

func (f *glfwProjectionFactory) Prepare() (bundle *nativeProjectionBundle, spec termmux.SpawnSpec, content termmux.PixelRect, metrics termmux.CellMetrics, title string, err error) {
	if f == nil || f.owner == nil || f.owner.controller == nil || f.owner.mux == nil {
		return nil, spec, content, metrics, "", errWindowProjectionMissing
	}
	child := newProjectionApp(f.owner)
	return f.prepareProjection(child, child.cfg.Window.Width, child.cfg.Window.Height, 0, 0, false, true)
}

func (f *glfwProjectionFactory) prepareProjection(child *App, width, height, x, y int, position, initialGrid bool) (bundle *nativeProjectionBundle, spec termmux.SpawnSpec, content termmux.PixelRect, metrics termmux.CellMetrics, title string, err error) {
	bundle = &nativeProjectionBundle{app: child, beforeUnbind: newCompositionBeforeUnbind(child)}
	fail := func(cause error) (*nativeProjectionBundle, termmux.SpawnSpec, termmux.PixelRect, termmux.CellMetrics, string, error) {
		return bundle, spec, content, metrics, "", cause
	}

	child.applyNativeWindowCreationHints()
	window, createErr := glfw.CreateWindow(width, height, "CervTerm", nil, nil)
	if createErr != nil {
		return fail(createErr)
	}
	bundle.host, child.window = window, window
	if position {
		window.SetPos(x, y)
	}
	window.MakeContextCurrent()
	if initErr := gl.Init(); initErr != nil {
		return fail(initErr)
	}
	child.transparentFramebuffer = window.GetAttrib(glfw.TransparentFramebuffer) == glfw.True
	child.blurProvider = newBlurProvider(window)
	bundle.resources = append(bundle.resources, projectionResourceFunc(func() error {
		child.closeDividerCursors()
		if child.blurProvider == nil {
			return nil
		}
		return child.blurProvider.Close()
	}))
	bundle.resources = append(bundle.resources, projectionResourceFunc(child.closeNotificationEffectSink))
	child.configureNativeWindow(window)
	child.applyWindowAppearance()
	glfw.SwapInterval(boolSwapInterval(child.cfg.Render.VSync))

	child.r = newGLRenderer(window)
	bundle.resources = append(bundle.resources, projectionResourceFunc(func() error {
		if child.r != nil {
			child.r.Destroy()
			child.r = nil
		}
		return nil
	}))
	if backgroundErr := child.prepareInitialBackgroundSurface(); backgroundErr != nil {
		return fail(backgroundErr)
	}
	bundle.resources = append(bundle.resources, projectionResourceFunc(func() error {
		child.closeBackgroundSurface()
		return nil
	}))

	sx, sy := window.GetContentScale()
	child.applyScale(sx, sy)
	plan, planErr := newStartupFontInstallationPlan(child.cfg, effectiveDPI(sx, sy), child.effectiveTextRaster(), child.safeFonts)
	if planErr != nil {
		return fail(planErr)
	}
	stages := defaultFontInstallationStages()
	prepared, prepareErr := prepareFontInstallation(plan, stages)
	if prepareErr != nil {
		return fail(prepareErr)
	}
	atlas, adoptErr := prepared.adopt(child.r, stages)
	prepared.Close()
	if adoptErr != nil {
		return fail(adoptErr)
	}
	child.atlas = atlas
	bundle.resources = append(bundle.resources, projectionResourceFunc(func() error {
		if child.atlas != nil {
			child.atlas.close()
			child.atlas = nil
		}
		return nil
	}))
	if imageErr := child.prepareTerminalImageCache(); imageErr != nil {
		return fail(imageErr)
	}
	appendTerminalImageCacheResource(bundle, child)
	child.ligaturesActive = atlas.supportsLigatures(child.cfg.Font.Ligatures)
	child.cellW, child.cellH = float32(atlas.cellW), float32(atlas.cellH)
	if initialGrid {
		if gridErr := child.applyInitialGridWindowPlan(window, sx, sy); gridErr != nil {
			return fail(gridErr)
		}
	}
	child.lastFBW, child.lastFBH = -1, -1
	fbW, fbH := window.GetFramebufferSize()
	content = child.muxContentBounds(fbW, fbH)
	metrics = child.muxMetrics()
	spec = termmux.SpawnSpec{Options: ptyio.Options{
		ShellProgram:     child.cfg.Shell.Program,
		ShellArgs:        append([]string(nil), child.cfg.Shell.Args...),
		WorkingDirectory: child.cfg.Shell.WorkingDirectory,
		Env:              cloneStringMap(child.cfg.Shell.Env),
	}}
	title = "CervTerm"
	bundle.handle = child.applyMuxEvents
	bundle.bind = func(id termmux.WindowID) error {
		if id == 0 {
			return fmt.Errorf("bind projection: %w", errWindowProjectionMissing)
		}
		child.windowID = id
		if accessibilityErr := prepareProjectionAccessibility(child, window, bundle.beforeUnbind); accessibilityErr != nil {
			child.windowID = 0
			return accessibilityErr
		}
		child.catchUpBellEvents()
		child.installCallbacks()
		child.needsRedraw = true
		return nil
	}
	bundle.unbind = func() error {
		child.windowID = 0
		return nil
	}
	child.activateProjectionIME(window, bundle.beforeUnbind)
	return bundle, spec, content, metrics, title, nil
}

func newProjectionApp(owner *App) *App {
	cfg := owner.projectionBase()
	child := &App{
		cfg: cfg, desiredCfg: owner.desiredCfg.Clone(), composedCfg: owner.composedCfg.Clone(),
		safeFonts: owner.safeFonts, composedProvenance: append([]config.ProvenanceRecord(nil), owner.composedProvenance...),
		configStateInitialized: true, configPath: owner.configPath, candidateOptions: owner.candidateOptions.Clone(),
		mux: owner.mux, controller: owner.controller, scriptRT: owner.scriptRT, scriptGeneration: owner.scriptGeneration,
		terminalImageCacheFactory: owner.terminalImageCacheFactory,
		cellW:                     9, cellH: 16, uiScale: 1, blinkStart: time.Now(),
		paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneScroll: make(map[termmux.PaneID]int),
		pendingPaneResize: make(map[termmux.PaneID]termmux.PaneGeometry), configWatchHashes: make(map[string][32]byte),
		bellState: bellState{bellDelivered: owner.bellDelivered},
	}
	child.initCompositionCoordinator()
	child.initZoomHotkeys()
	child.initActionController()
	child.initInputController()
	child.initRenderController()
	child.initReloadController()
	child.initActionBindings()
	if spec, ok := parseStatsHotkey(cfg.Render.StatsHotkey); ok {
		child.statsSpec, child.statsSpecOK = spec, true
	}
	return child
}

func boolSwapInterval(vsync bool) int {
	if vsync {
		return 1
	}
	return 0
}

var _ nativeProjectionCandidateFactory = (*glfwProjectionFactory)(nil)
