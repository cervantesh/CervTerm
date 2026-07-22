//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/frontend/gpu"
	termmux "cervterm/internal/mux"
	"cervterm/internal/termimage"
)

const (
	terminalImageKittyMask = 1 << iota
	terminalImageSixelMask
	terminalImageITermMask
)

func terminalImageConfig(mask int) config.Config {
	cfg := config.Defaults()
	cfg.Graphics.Kitty.Enabled = mask&terminalImageKittyMask != 0
	cfg.Graphics.Sixel.Enabled = mask&terminalImageSixelMask != 0
	cfg.Graphics.ITerm.Enabled = mask&terminalImageITermMask != 0
	cfg.Graphics.Limits.EncodedBytesPerPane = 1024
	cfg.Graphics.Limits.DecodedBytesPerPane = 2048
	cfg.Graphics.Limits.ImageCountPerPane = 3
	cfg.Graphics.Limits.PlacementCountPerPane = 4
	cfg.Graphics.Limits.GPUBytesPerContext = 4096
	return cfg
}

func enabledTerminalImageConfig() config.Config {
	return terminalImageConfig(terminalImageKittyMask | terminalImageSixelMask | terminalImageITermMask)
}

func isolatedTerminalImageConfigs() []struct {
	name string
	cfg  config.Config
} {
	return []struct {
		name string
		cfg  config.Config
	}{
		{name: "sixel-only", cfg: terminalImageConfig(terminalImageSixelMask)},
		{name: "iterm-only", cfg: terminalImageConfig(terminalImageITermMask)},
	}
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

func TestTerminalImageActivationAllProtocolMasks(t *testing.T) {
	wantLimits := termimage.Limits{EncodedBytes: 1024, DecodedBytes: 2048, Images: 3, Placements: 4}
	for mask := 0; mask < 8; mask++ {
		t.Run(fmt.Sprintf("mask-%d", mask), func(t *testing.T) {
			cfg := terminalImageConfig(mask)
			probe := &terminalImageCacheFactoryProbe{}
			app := &App{cfg: cfg, r: &glRenderer{}, terminalImageCacheFactory: probe.create}
			options := app.muxOptions()
			kitty := mask&terminalImageKittyMask != 0
			sixel := mask&terminalImageSixelMask != 0
			iterm := mask&terminalImageITermMask != 0
			enabled := kitty || sixel || iterm
			if options.KittyEnabled != kitty || options.SixelEnabled != sixel || options.ITermEnabled != iterm {
				t.Fatalf("image options=%#v", options)
			}
			if !enabled {
				if options.ImageLimits != nil {
					t.Fatalf("all-disabled image limits=%#v", options.ImageLimits)
				}
			} else if options.ImageLimits == nil || *options.ImageLimits != wantLimits {
				t.Fatalf("image limits=%#v want=%#v", options.ImageLimits, wantLimits)
			}
			if err := app.initMux(); err != nil {
				t.Fatal(err)
			}
			if err := app.prepareTerminalImageCache(); err != nil {
				t.Fatal(err)
			}
			expectedCalls := 0
			if enabled {
				expectedCalls = 1
			}
			if (app.terminalImageCache != nil) != enabled || len(probe.calls) != expectedCalls {
				t.Fatalf("cache=%p factory calls=%d enabled=%v", app.terminalImageCache, len(probe.calls), enabled)
			}
			if enabled {
				call := probe.calls[0]
				if call.renderer != app.r.(gpu.TerminalImageRenderer) || call.limits != (terminalImageCacheLimits{Entries: termimage.HardGPUEntriesPerContext, Bytes: 4096}) {
					t.Fatalf("cache factory call=%#v", call)
				}
			} else if app.terminalImages.panes != nil || app.terminalImages.draws != nil || app.terminalImages.candidates != nil || app.terminalImages.keys != nil || app.terminalImages.seen != nil || app.terminalImageDamage.panes != nil {
				t.Fatalf("all-disabled draw state=%#v damage=%#v", app.terminalImages, app.terminalImageDamage)
			}
			if deadline, ok := app.mux.NextImageDeadline(); ok || !deadline.IsZero() {
				t.Fatalf("idle mux advertised image work deadline=%v ok=%v", deadline, ok)
			}
			if retry, ok := app.terminalImageCache.nextRetryDeadline(); ok || !retry.IsZero() {
				t.Fatalf("idle cache advertised retry=%v ok=%v", retry, ok)
			}
			if err := app.closeTerminalImageCache(); err != nil {
				t.Fatal(err)
			}
			if err := app.rollbackInitializedMux(nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTerminalImageActivationMapsEnabledLimitsIntoOneMux(t *testing.T) {
	cfg := enabledTerminalImageConfig()
	app := &App{cfg: cfg}
	options := app.muxOptions()
	if !options.KittyEnabled || !options.SixelEnabled || !options.ITermEnabled || options.ImageLimits == nil {
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
	for _, test := range isolatedTerminalImageConfigs() {
		t.Run(test.name, func(t *testing.T) {
			app := &App{cfg: test.cfg, r: &atlasTestRenderer{}}
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
		})
	}
}

func TestTerminalImageActivationCreatesInitialChildAndRestoreCaches(t *testing.T) {
	cfg := enabledTerminalImageConfig()
	future := terminalImageConfig(0)
	future.Graphics.Limits.GPUBytesPerContext = 8192
	probe := &terminalImageCacheFactoryProbe{}
	owner := &App{
		cfg: cfg, desiredCfg: future, composedCfg: future,
		terminalImageCacheFactory: probe.create,
	}
	pendingProjectionBase := future.Clone()
	owner.projectionBaseConfig = &pendingProjectionBase
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
		if index > 0 && call.renderer == probe.calls[0].renderer {
			t.Fatalf("cache %d reused renderer/context identity", index)
		}
	}
	if child.cfg.Graphics != cfg.Graphics || restored.cfg.Graphics != cfg.Graphics {
		t.Fatalf("child used pending graphics config child=%#v restored=%#v", child.cfg.Graphics, restored.cfg.Graphics)
	}
	if err := child.prepareTerminalImageCache(); err == nil || len(probe.calls) != 3 {
		t.Fatal("child created a second cache")
	}
	ownerCache, childCache, restoredCache := owner.terminalImageCache, child.terminalImageCache, restored.terminalImageCache
	if err := child.closeTerminalImageCache(); err != nil {
		t.Fatal(err)
	}
	if !childCache.closed || ownerCache.closed || restoredCache.closed || owner.terminalImageCache != ownerCache || restored.terminalImageCache != restoredCache {
		t.Fatal("closing one projection cache affected another context")
	}
	if err := restored.closeTerminalImageCache(); err != nil {
		t.Fatal(err)
	}
	if !restoredCache.closed || ownerCache.closed || owner.terminalImageCache != ownerCache {
		t.Fatal("closing restored cache affected owner context")
	}
	if err := owner.closeTerminalImageCache(); err != nil {
		t.Fatal(err)
	}
	if err := owner.rollbackInitializedMux(nil); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalImageActivationPendingEnableCannotReachChildOrRestoredProjection(t *testing.T) {
	effective := terminalImageConfig(0)
	pending := terminalImageConfig(terminalImageSixelMask | terminalImageITermMask)
	probe := &terminalImageCacheFactoryProbe{}
	owner := &App{
		cfg: effective, desiredCfg: pending, composedCfg: pending,
		terminalImageCacheFactory: probe.create,
	}
	pendingProjectionBase := pending.Clone()
	owner.projectionBaseConfig = &pendingProjectionBase
	if err := owner.initMux(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"child", "restored"} {
		t.Run(name, func(t *testing.T) {
			projection := newProjectionApp(owner)
			projection.r = &atlasTestRenderer{}
			if projection.cfg.Graphics != effective.Graphics {
				t.Fatalf("pending graphics leaked into %s: %#v", name, projection.cfg.Graphics)
			}
			if err := projection.prepareTerminalImageCache(); err != nil {
				t.Fatalf("all-disabled %s required renderer capability: %v", name, err)
			}
			if projection.terminalImageCache != nil {
				t.Fatalf("pending graphics activated %s cache", name)
			}
		})
	}
	if len(probe.calls) != 0 {
		t.Fatalf("pending graphics reached cache factory %d times", len(probe.calls))
	}
	if err := owner.rollbackInitializedMux(nil); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalImageActivationFactoryErrorClosesCandidateAndRollsBackStartupMux(t *testing.T) {
	for _, test := range isolatedTerminalImageConfigs() {
		t.Run(test.name, func(t *testing.T) {
			key := cacheKey(1, 1, 1)
			candidate, renderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 1, Bytes: 4}, map[gpu.ImageTextureKey]termimage.DetachedResource{key: cacheResource(key, 4)})
			if result := candidate.beginFrame(cacheTestTime(0), []gpu.ImageTextureKey{key}); result.Ready != 1 {
				t.Fatal(result)
			}
			injected := errors.New("injected cache factory failure")
			probe := &terminalImageCacheFactoryProbe{returnCache: candidate, returnErr: injected}
			app := &App{cfg: test.cfg, r: &glRenderer{}, terminalImageCacheFactory: probe.create}
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
		})
	}
}

func TestTerminalImageActivationCommitFailureClosesCacheThenMux(t *testing.T) {
	for _, test := range isolatedTerminalImageConfigs() {
		t.Run(test.name, func(t *testing.T) {
			injected := errors.New("commit")
			probe := &terminalImageCacheFactoryProbe{}
			app := &App{cfg: test.cfg, r: &glRenderer{}, terminalImageCacheFactory: probe.create}
			if err := app.activateInitialTerminalImages(func() error { return injected }); !errors.Is(err, injected) {
				t.Fatalf("err=%v", err)
			}
			if app.terminalImageCache != nil || app.mux != nil || len(probe.calls) != 1 || probe.calls[0].cache == nil || !probe.calls[0].cache.closed {
				t.Fatalf("rollback cache=%p mux=%p calls=%#v", app.terminalImageCache, app.mux, probe.calls)
			}
		})
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
	if graphics != 8 {
		t.Fatalf("graphics restart changes=%d want=8 (%#v)", graphics, changes)
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

func TestTerminalImageActivationRuntimePrepareAndBindFailuresCloseCacheInCurrentContext(t *testing.T) {
	for _, stage := range []string{"prepare", "bind"} {
		t.Run(stage, func(t *testing.T) {
			var log []string
			cache := newEmptyActivationCache(t)
			app := &App{terminalImageCache: cache}
			host := &fakeNativeWindow{id: "image-child", log: &log}
			injected := errors.New(stage)
			factory := &fakeCandidateFactory{log: &log, host: host, app: app, resource: projectionResourceFunc(func() error {
				log = append(log, "close:image-cache")
				return app.closeTerminalImageCache()
			})}
			if stage == "prepare" {
				factory.prepare = injected
			} else {
				factory.bind = injected
			}
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
			want := []string{"prepare-native", "current:image-child", "close:image-cache", "destroy:image-child"}
			wantClosed := 0
			if stage == "bind" {
				want = []string{"prepare-native", "create-runtime", "bind:2", "rollback-runtime:2", "current:image-child", "close:image-cache", "destroy:image-child"}
				wantClosed = 1
			}
			if !reflect.DeepEqual(log, want) {
				t.Fatalf("rollback order=%v want=%v", log, want)
			}
			if !cache.closed || app.terminalImageCache != nil || runtimeWindows.closed != wantClosed || host.destroyed != 1 || len(controller.windows) != 0 {
				t.Fatalf("cache=%v appcache=%p closed=%d host=%d windows=%d", cache.closed, app.terminalImageCache, runtimeWindows.closed, host.destroyed, len(controller.windows))
			}
		})
	}
}

func TestTerminalImageActivationRuntimeFocusAndClosePreserveContextOwnership(t *testing.T) {
	var log []string
	cache := newEmptyActivationCache(t)
	app := &App{terminalImageCache: cache}
	host := &fakeNativeWindow{id: "image-child", log: &log}
	factory := &fakeCandidateFactory{log: &log, host: host, app: app, resource: projectionResourceFunc(func() error {
		log = append(log, "close:image-cache")
		return app.closeTerminalImageCache()
	})}
	runtimeWindows := &fakeRuntimeWindows{log: &log}
	controller := newWindowController(processServices{}, fakeNativePump{log: &log})
	controller.setCandidateFactory(factory)
	controller.setRuntimeWindows(runtimeWindows)
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	id, err := controller.createRuntimeProjection()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.closeRuntimeProjection(id); err != nil {
		t.Fatal(err)
	}
	want := []string{"prepare-native", "create-runtime", "bind:2", "focus:image-child", "close-runtime:2", "current:image-child", "close:image-cache", "destroy:image-child"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("focus/close order=%v want=%v", log, want)
	}
	if !cache.closed || app.terminalImageCache != nil {
		t.Fatal("focused child retained cache after current-context close")
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
			factory.resourceFor = func(index int, app *App) projectionResource {
				cache := newEmptyActivationCache(t)
				caches = append(caches, cache)
				app.terminalImageCache = cache
				return projectionResourceFunc(func() error {
					log = append(log, fmt.Sprintf("cache:%d", index))
					return app.closeTerminalImageCache()
				})
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
				if abortErr := controller.abortRestoreProjections(candidate, nil); abortErr != nil {
					t.Fatal(abortErr)
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
			var want []string
			if bindFailure {
				want = []string{
					"prepare:0", "hide:restore-0", "prepare:1", "hide:restore-1", "prepare:2", "hide:restore-2",
					"bind:0:2", "bind:1:3", "unbind:1", "unbind:0",
					"current:restore-2", "unbind:2", "cache:2", "close:2", "destroy:restore-2",
					"current:restore-1", "cache:1", "close:1", "destroy:restore-1",
					"current:restore-0", "cache:0", "close:0", "destroy:restore-0",
				}
			} else {
				want = []string{
					"prepare:0", "hide:restore-0", "prepare:1",
					"current:restore-1", "unbind:1", "cache:1", "close:1", "destroy:restore-1",
					"current:restore-0", "unbind:0", "cache:0", "close:0", "destroy:restore-0",
				}
			}
			if !reflect.DeepEqual(log, want) {
				t.Fatalf("rollback order=%v want=%v", log, want)
			}
		})
	}
}

var _ terminalImageCacheFactory = (*terminalImageCacheFactoryProbe)(nil).create
var _ = termmux.PaneID(0)
