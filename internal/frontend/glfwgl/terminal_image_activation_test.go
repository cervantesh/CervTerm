//go:build glfw

package glfwgl

import (
	"errors"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/frontend/gpu"
	termmux "cervterm/internal/mux"
	"cervterm/internal/termimage"
)

func enabledTerminalImageConfig() config.Config {
	cfg := config.Defaults()
	cfg.Graphics.Kitty.Enabled = true
	cfg.Graphics.Limits.EncodedBytesPerPane = 1024
	cfg.Graphics.Limits.DecodedBytesPerPane = 2048
	cfg.Graphics.Limits.ImageCountPerPane = 3
	cfg.Graphics.Limits.PlacementCountPerPane = 4
	cfg.Graphics.Limits.GPUBytesPerContext = 4096
	return cfg
}

type terminalImageCacheFactoryCall struct {
	renderer gpu.TerminalImageRenderer
	acquire  terminalImageAcquire
	limits   terminalImageCacheLimits
	cache    *terminalImageCache
}

type terminalImageCacheFactoryProbe struct {
	calls       []terminalImageCacheFactoryCall
	returnCache *terminalImageCache
	returnErr   error
}

func (p *terminalImageCacheFactoryProbe) create(renderer gpu.TerminalImageRenderer, acquire terminalImageAcquire, limits terminalImageCacheLimits) (*terminalImageCache, error) {
	call := terminalImageCacheFactoryCall{renderer: renderer, acquire: acquire, limits: limits}
	if p.returnCache != nil || p.returnErr != nil {
		call.cache = p.returnCache
		p.calls = append(p.calls, call)
		return p.returnCache, p.returnErr
	}
	cache, err := newTerminalImageCache(renderer, acquire, limits)
	call.cache = cache
	p.calls = append(p.calls, call)
	return cache, err
}

func TestTerminalImageActivationDisabledIsLiteralNil(t *testing.T) {
	for _, name := range []string{"default", "v1-effective", "explicit-disabled"} {
		t.Run(name, func(t *testing.T) {
			cfg := config.Defaults()
			if name == "explicit-disabled" {
				cfg.Graphics.Limits.GPUBytesPerContext = 1024
			}
			probe := &terminalImageCacheFactoryProbe{}
			app := &App{cfg: cfg, r: &glRenderer{}, terminalImageCacheFactory: probe.create}
			options := app.muxOptions()
			if options.ImageLimits != nil || options.KittyEnabled {
				t.Fatalf("disabled image options=%#v", options)
			}
			if err := app.initMux(); err != nil {
				t.Fatal(err)
			}
			if err := app.prepareTerminalImageCache(); err != nil {
				t.Fatal(err)
			}
			if app.terminalImageCache != nil || len(probe.calls) != 0 {
				t.Fatalf("disabled cache=%p factory calls=%d", app.terminalImageCache, len(probe.calls))
			}
			if deadline, ok := app.mux.NextImageDeadline(); ok || !deadline.IsZero() {
				t.Fatalf("disabled mux advertised image work deadline=%v ok=%v", deadline, ok)
			}
			if err := app.rollbackInitializedMux(nil); err != nil {
				t.Fatal(err)
			}
			if app.mux != nil {
				t.Fatal("disabled mux owner remained published after rollback")
			}
		})
	}
}

func TestTerminalImageActivationMapsEnabledLimitsIntoOneMux(t *testing.T) {
	cfg := enabledTerminalImageConfig()
	app := &App{cfg: cfg}
	options := app.muxOptions()
	if !options.KittyEnabled || options.ImageLimits == nil {
		t.Fatalf("enabled image options=%#v", options)
	}
	want := termimage.Limits{EncodedBytes: 1024, DecodedBytes: 2048, Images: 3, Placements: 4}
	if *options.ImageLimits != want {
		t.Fatalf("mux limits=%#v want=%#v", *options.ImageLimits, want)
	}
	if err := app.initMux(); err != nil {
		t.Fatal(err)
	}
	published := app.mux
	if published == nil || published.ImageSetupError() != nil {
		t.Fatalf("published mux=%p setup=%v", published, published.ImageSetupError())
	}
	if err := app.initMux(); err == nil {
		t.Fatal("second mux initialization succeeded")
	}
	if app.mux != published {
		t.Fatal("second mux initialization replaced the shared owner")
	}
	if err := app.rollbackInitializedMux(nil); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalImageActivationRejectsMuxSetupAtomically(t *testing.T) {
	cfg := enabledTerminalImageConfig()
	cfg.Graphics.Limits.EncodedBytesPerPane = 0
	app := &App{cfg: cfg}
	if err := app.initMux(); err == nil {
		t.Fatal("invalid image limits initialized a mux")
	}
	if app.mux != nil || app.terminalImageCache != nil {
		t.Fatalf("failed setup published mux=%p cache=%p", app.mux, app.terminalImageCache)
	}
}

func TestTerminalImageActivationFailsClosedWithoutRendererCapability(t *testing.T) {
	app := &App{cfg: enabledTerminalImageConfig(), r: &atlasTestRenderer{}}
	if err := app.initMux(); err != nil {
		t.Fatal(err)
	}
	if err := app.prepareTerminalImageCache(); err == nil {
		t.Fatal("enabled activation accepted renderer without image capability")
	}
	if app.terminalImageCache != nil {
		t.Fatal("unsupported renderer published cache")
	}
	if err := app.rollbackInitializedMux(nil); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalImageActivationCreatesInitialChildAndRestoreCaches(t *testing.T) {
	cfg := enabledTerminalImageConfig()
	future := cfg.Clone()
	future.Graphics.Kitty.Enabled = false
	future.Graphics.Limits.GPUBytesPerContext = 8192
	probe := &terminalImageCacheFactoryProbe{}
	owner := &App{
		cfg: cfg, desiredCfg: future, composedCfg: future,
		terminalImageCacheFactory: probe.create,
	}
	if err := owner.initMux(); err != nil {
		t.Fatal(err)
	}
	owner.r = &glRenderer{}
	if err := owner.prepareTerminalImageCache(); err != nil {
		t.Fatal(err)
	}

	child := newProjectionApp(owner)
	child.r = &glRenderer{}
	if err := child.prepareTerminalImageCache(); err != nil {
		t.Fatal(err)
	}
	restored := newProjectionApp(owner)
	restored.r = &glRenderer{}
	if err := restored.prepareTerminalImageCache(); err != nil {
		t.Fatal(err)
	}

	if len(probe.calls) != 3 {
		t.Fatalf("cache factory calls=%d want=3", len(probe.calls))
	}
	if owner.terminalImageCache == nil || child.terminalImageCache == nil || restored.terminalImageCache == nil ||
		owner.terminalImageCache == child.terminalImageCache || child.terminalImageCache == restored.terminalImageCache {
		t.Fatal("projection caches are missing or shared")
	}
	for index, call := range probe.calls {
		if call.limits.Entries != termimage.HardGPUEntriesPerContext || call.limits.Bytes != cfg.Graphics.Limits.GPUBytesPerContext {
			t.Fatalf("cache %d limits=%#v", index, call.limits)
		}
		key := gpu.ImageTextureKey{PaneObject: 99, Resource: termimage.ResourceRef{Image: 1, Generation: 1}}
		if _, ok := call.acquire(key); ok {
			t.Fatalf("cache %d acquired unpublished shared mux resource", index)
		}
	}
	if child.cfg.Graphics != cfg.Graphics || restored.cfg.Graphics != cfg.Graphics {
		t.Fatalf("child used pending graphics config child=%#v restored=%#v", child.cfg.Graphics, restored.cfg.Graphics)
	}
	if err := child.prepareTerminalImageCache(); err == nil || len(probe.calls) != 3 {
		t.Fatal("child created a second cache")
	}
	for _, app := range []*App{restored, child, owner} {
		if err := app.closeTerminalImageCache(); err != nil {
			t.Fatal(err)
		}
	}
	if err := owner.rollbackInitializedMux(nil); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalImageActivationFactoryErrorClosesCandidateAndRollsBackStartupMux(t *testing.T) {
	key := cacheKey(1, 1, 1)
	candidate, renderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 1, Bytes: 4}, map[gpu.ImageTextureKey]termimage.DetachedResource{key: cacheResource(key, 4)})
	if result := candidate.beginFrame(cacheTestTime(0), []gpu.ImageTextureKey{key}); result.Ready != 1 {
		t.Fatal(result)
	}
	injected := errors.New("injected cache factory failure")
	probe := &terminalImageCacheFactoryProbe{returnCache: candidate, returnErr: injected}
	app := &App{cfg: enabledTerminalImageConfig(), r: &glRenderer{}, terminalImageCacheFactory: probe.create}
	if err := app.initMux(); err != nil {
		t.Fatal(err)
	}
	if err := app.prepareTerminalImageCache(); !errors.Is(err, injected) {
		t.Fatalf("factory error=%v", err)
	}
	if app.terminalImageCache != nil || renderer.textures[0].closed != 1 {
		t.Fatalf("failed factory published cache=%p texture closes=%d", app.terminalImageCache, renderer.textures[0].closed)
	}
	if err := app.rollbackInitializedMux(injected); !errors.Is(err, injected) {
		t.Fatalf("rollback error=%v", err)
	}
	if app.mux != nil {
		t.Fatal("failed startup retained mux budget/scheduler owner")
	}
}

func TestTerminalImageActivationCommitFailureClosesCacheThenMux(t *testing.T) {
	injected := errors.New("commit")
	probe := &terminalImageCacheFactoryProbe{}
	app := &App{cfg: enabledTerminalImageConfig(), r: &glRenderer{}, terminalImageCacheFactory: probe.create}
	if err := app.activateInitialTerminalImages(func() error { return injected }); !errors.Is(err, injected) {
		t.Fatalf("err=%v", err)
	}
	if app.terminalImageCache != nil || app.mux != nil || len(probe.calls) != 1 || probe.calls[0].cache == nil || !probe.calls[0].cache.closed {
		t.Fatalf("rollback cache=%p mux=%p calls=%#v", app.terminalImageCache, app.mux, probe.calls)
	}
}

func TestTerminalImageActivationConfigDiffRemainsRestartScoped(t *testing.T) {
	effective := config.Defaults()
	desired := enabledTerminalImageConfig()
	changes := config.DiffConfig(desired, effective)
	graphics := 0
	for _, change := range changes {
		if len(change.Path) >= len("graphics.") && change.Path[:len("graphics.")] == "graphics." {
			graphics++
			if change.Scope != config.ApplyRestart {
				t.Fatalf("graphics change %#v is not restart scoped", change)
			}
		}
	}
	if graphics != 6 {
		t.Fatalf("graphics restart changes=%d want=6 (%#v)", graphics, changes)
	}
}

func TestAppendTerminalImageCacheResourceSkipsNilAndClosesOwnedCache(t *testing.T) {
	app := &App{}
	bundle := &nativeProjectionBundle{}
	appendTerminalImageCacheResource(bundle, app)
	if len(bundle.resources) != 0 {
		t.Fatal("nil cache registered a projection owner")
	}
	cache, err := newTerminalImageCache(&fakeCacheRenderer{failures: make(map[gpu.ImageTextureKey]int)}, func(gpu.ImageTextureKey) (termimage.DetachedResource, bool) {
		return termimage.DetachedResource{}, false
	}, terminalImageCacheLimits{Entries: 1, Bytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	app.terminalImageCache = cache
	appendTerminalImageCacheResource(bundle, app)
	if len(bundle.resources) != 1 {
		t.Fatalf("registered resources=%d", len(bundle.resources))
	}
	if err := bundle.close(); err != nil {
		t.Fatal(err)
	}
	if app.terminalImageCache != nil || !cache.closed {
		t.Fatal("projection close retained cache")
	}
}

func newEmptyActivationCache(t *testing.T) *terminalImageCache {
	t.Helper()
	cache, err := newTerminalImageCache(&fakeCacheRenderer{failures: make(map[gpu.ImageTextureKey]int)}, func(gpu.ImageTextureKey) (termimage.DetachedResource, bool) {
		return termimage.DetachedResource{}, false
	}, terminalImageCacheLimits{Entries: 1, Bytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func TestTerminalImageActivationRuntimeBindFailureClosesCacheAndMuxWindow(t *testing.T) {
	var log []string
	cache := newEmptyActivationCache(t)
	app := &App{terminalImageCache: cache}
	host := &fakeNativeWindow{id: "image-child", log: &log}
	injected := errors.New("bind")
	factory := &fakeCandidateFactory{log: &log, bind: injected, host: host, app: app, resource: projectionResourceFunc(app.closeTerminalImageCache)}
	runtimeWindows := &fakeRuntimeWindows{log: &log}
	controller := newWindowController(processServices{}, fakeNativePump{log: &log})
	controller.setCandidateFactory(factory)
	controller.setRuntimeWindows(runtimeWindows)
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.createRuntimeProjection(); !errors.Is(err, injected) {
		t.Fatalf("err=%v", err)
	}
	if !cache.closed || app.terminalImageCache != nil || runtimeWindows.closed != 1 || host.destroyed != 1 || len(controller.windows) != 0 {
		t.Fatalf("cache=%v appcache=%p closed=%d host=%d windows=%d", cache.closed, app.terminalImageCache, runtimeWindows.closed, host.destroyed, len(controller.windows))
	}
}

func TestTerminalImageActivationRestorePreparationAndBindFailuresCloseEveryCache(t *testing.T) {
	for _, bindFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepare", true: "bind"}[bindFailure], func(t *testing.T) {
			var log []string
			var caches []*terminalImageCache
			factory := &fakeRestoreProjectionFactory{log: &log, failAt: 1, bindAt: -1}
			if bindFailure {
				factory.failAt = -1
				factory.bindAt = 1
			}
			factory.resourceFor = func(_ int, app *App) projectionResource {
				cache := newEmptyActivationCache(t)
				caches = append(caches, cache)
				app.terminalImageCache = cache
				return projectionResourceFunc(app.closeTerminalImageCache)
			}
			controller := newRestoreProjectionController(t, &log)
			candidate, err := controller.prepareRestoreProjections(factory, 3)
			if bindFailure {
				if err != nil {
					t.Fatal(err)
				}
				if err = controller.publishRestoreProjections(candidate, []termmux.WindowID{2, 3, 4}); err == nil {
					t.Fatal("bind failure accepted")
				}
			} else if candidate != nil || err == nil {
				t.Fatalf("candidate=%p err=%v", candidate, err)
			}
			for i, cache := range caches {
				if !cache.closed || factory.apps[i].terminalImageCache != nil {
					t.Fatalf("cache %d leaked closed=%v app=%p", i, cache.closed, factory.apps[i].terminalImageCache)
				}
			}
			assertRestoreProjectionPristine(t, controller, factory.hosts)
		})
	}
}

var _ terminalImageCacheFactory = (*terminalImageCacheFactoryProbe)(nil).create
var _ = termmux.PaneID(0)
