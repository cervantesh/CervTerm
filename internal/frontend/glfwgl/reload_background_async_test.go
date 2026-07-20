//go:build glfw

package glfwgl

import (
	"errors"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"
)

func newAsyncReloadApp(t *testing.T, path string) *App {
	t.Helper()
	loaded, err := script.LoadVersioned(path, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	activation, err := loaded.Candidate.PrepareActivation()
	if err != nil {
		loaded.Candidate.Close()
		t.Fatal(err)
	}
	app := &App{
		cfg:                    loaded.Config,
		desiredCfg:             loaded.Config.Clone(),
		composedCfg:            loaded.Config.Clone(),
		configStateInitialized: true,
		scriptRT:               activation.Commit(),
		scriptBundle:           loaded.Candidate,
		configPath:             path,
		candidateOptions:       loaded.Options,
		configWatch:            newConfigWatchState(loaded.WatchPaths...),
		mux:                    termmux.New(nil, termmux.Options{}),
		paneUI:                 make(map[termmux.PaneID]*paneUIState),
		r:                      &replaceRecordingRenderer{},
		lastFBW:                8,
		lastFBH:                6,
	}
	t.Cleanup(func() {
		if app.scriptBundle != nil {
			app.scriptBundle.Close()
		} else if app.scriptRT != nil {
			app.scriptRT.Close()
		}
		app.closeBackgroundSurface()
	})
	return app
}

func driveAsyncReload(t *testing.T, app *App, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() && time.Now().Before(deadline) {
		app.applyPendingConfigReload()
		time.Sleep(time.Millisecond)
	}
	if !condition() {
		t.Fatalf("async reload did not reach expected state: workers=%d generation=%d queued=%d pending=%v error=%q layers=%d watches=%v", app.configReloadAsync.workers, app.configReloadAsync.generation, len(app.configReloadAsync.results), app.reloadPending, app.lastConfigReloadError, len(app.cfg.Background.Layers), app.configWatch.activePaths)
	}
}

func TestLayeredConfigReloadRunsCPUGenerationOffMainAndTransfersAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cervterm.lua")
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"}}`)
	app := newAsyncReloadApp(t, path)
	oldBundle := app.scriptBundle
	oldSurface := &recordingBackgroundSurface{}
	app.backgroundSurface = oldSurface
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"},background={layers={{kind="linear_gradient",colors={"#000000","#800080"},angle=90}}}}`)
	app.reloadPending = true
	started := time.Now()
	app.applyPendingConfigReload()
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("main-thread dispatch blocked for %v", elapsed)
	}
	if app.configReloadAsync.workers == 0 {
		t.Fatal("reload worker was not started")
	}
	if app.scriptBundle != oldBundle {
		t.Fatal("active bundle changed before worker completion")
	}
	driveAsyncReload(t, app, func() bool { return len(app.cfg.Background.Layers) == 1 && app.configReloadAsync.workers == 0 })
	if app.backgroundSurface == nil || app.scriptBundle == oldBundle {
		t.Fatal("config, GPU surface, and script bundle were not transferred together")
	}
	if oldSurface.closed != 1 {
		t.Fatalf("old background close count = %d", oldSurface.closed)
	}
}

func TestStaleConfigReloadCPUGenerationCannotActivate(t *testing.T) {
	app := &App{cfg: config.Defaults()}
	app.configReloadAsync.generation = 2
	app.configReloadAsync.workers = 1
	app.configReloadAsync.results = make(chan *PreparedAppearanceGeneration, 1)
	generation := &PreparedAppearanceGeneration{configReloadCPUResult: configReloadCPUResult{generation: 1, loaded: script.VersionedSource{Config: config.Defaults()}}, state: appearancePreparedCPU}
	app.configReloadAsync.results <- generation
	app.applyConfigReloadWorkerResults()
	if app.configReloadAsync.workers != 0 {
		t.Fatalf("workers = %d", app.configReloadAsync.workers)
	}
	if app.configReloadAsync.prepared != nil {
		t.Fatal("stale generation became activatable")
	}
	if !generation.closed || generation.state != appearanceClosed {
		t.Fatalf("stale generation lifecycle: closed=%v state=%d", generation.closed, generation.state)
	}
	generation.Close()
	if generation.state != appearanceClosed {
		t.Fatal("generation close was not idempotent")
	}
}

func TestFailedBackgroundImageJoinsRetryWatchSetWithoutChangingActiveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cervterm.lua")
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"}}`)
	app := newAsyncReloadApp(t, path)
	oldBundle := app.scriptBundle
	missing := filepath.Join(dir, "missing.png")
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"},background={layers={{kind="image",path="missing.png"}}}}`)
	app.reloadPending = true
	driveAsyncReload(t, app, func() bool { return app.lastConfigReloadError != "" && app.configReloadAsync.workers == 0 })
	if app.scriptBundle != oldBundle || len(app.cfg.Background.Layers) != 0 {
		t.Fatal("failed image generation changed active state")
	}
	if !containsString(app.configWatch.failedPaths, missing) {
		t.Fatalf("failed watch paths = %v, want %q", app.configWatch.failedPaths, missing)
	}
	app.reloadPending = false
	writeBackgroundPNG(t, dir, "missing.png", color.RGBA{G: 255, A: 255})
	base := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	if app.configWatch.poll(base) {
		t.Fatal("failed image creation fired before debounce")
	}
	if !app.configWatch.poll(base.Add(300 * time.Millisecond)) {
		t.Fatal("failed image creation did not trigger retry")
	}
	app.reloadPending = true
	driveAsyncReload(t, app, func() bool { return len(app.cfg.Background.Layers) == 1 && app.configReloadAsync.workers == 0 })
	if len(app.configWatch.failedPaths) != 0 {
		t.Fatalf("failed paths survived recovery: %v", app.configWatch.failedPaths)
	}
}

func TestBackgroundGPUPreparationFailureRollsBackWholeReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cervterm.lua")
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"}}`)
	app := newAsyncReloadApp(t, path)
	oldBundle, oldConfig := app.scriptBundle, app.cfg.Clone()
	oldSurface := &recordingBackgroundSurface{}
	app.backgroundSurface = oldSurface
	app.r.(*replaceRecordingRenderer).prepareErr = errors.New("injected GPU failure")
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"},background={layers={{kind="linear_gradient",colors={"#000000","#ffffff"}}}}}`)
	app.reloadPending = true
	driveAsyncReload(t, app, func() bool { return app.lastConfigReloadError != "" && app.configReloadAsync.workers == 0 })
	if app.scriptBundle != oldBundle || len(app.cfg.Background.Layers) != len(oldConfig.Background.Layers) || app.backgroundSurface != oldSurface {
		t.Fatal("GPU preparation failure partially activated config, script, or surface")
	}
	if oldSurface.closed != 0 {
		t.Fatalf("active background closed on rollback: %d", oldSurface.closed)
	}
}

func TestSuccessfulBackgroundImageBecomesActiveWatchDependency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cervterm.lua")
	imagePath := writeBackgroundPNG(t, dir, "wall.png", color.RGBA{B: 255, A: 255})
	imagePath, _ = filepath.EvalSymlinks(imagePath)
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"}}`)
	app := newAsyncReloadApp(t, path)
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"},background={layers={{kind="image",path="wall.png",fit="stretch"}}}}`)
	app.reloadPending = true
	driveAsyncReload(t, app, func() bool { return len(app.cfg.Background.Layers) == 1 && app.configReloadAsync.workers == 0 })
	found := false
	for _, watched := range app.configWatch.activePaths {
		found = found || strings.EqualFold(watched, imagePath)
	}
	if !found {
		t.Fatalf("active watch paths = %v, want image %q", app.configWatch.activePaths, imagePath)
	}
}

func TestImageDigestChangeBeforeActivationDiscardsPreparedGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cervterm.lua")
	writeBackgroundPNG(t, dir, "wall.png", color.RGBA{R: 255, A: 255})
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"}}`)
	app := newAsyncReloadApp(t, path)
	oldBundle := app.scriptBundle
	writeReloadConfig(t, path, `return {config_version=2,colors={background="#080B12"},background={layers={{kind="image",path="wall.png",fit="stretch"}}}}`)
	app.startConfigReloadWorker()
	deadline := time.Now().Add(3 * time.Second)
	for len(app.configReloadAsync.results) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(app.configReloadAsync.results) == 0 {
		t.Fatal("worker result was not queued")
	}
	writeBackgroundPNG(t, dir, "wall.png", color.RGBA{G: 255, A: 255})
	app.applyConfigReloadWorkerResults()
	if app.scriptBundle != oldBundle || len(app.cfg.Background.Layers) != 0 || app.backgroundSurface != nil {
		t.Fatal("snapshot-stale image generation partially activated")
	}
	if !app.reloadPending || !strings.Contains(app.lastConfigReloadError, "changed while preparing") {
		t.Fatalf("stale generation retry/error: pending=%v error=%q", app.reloadPending, app.lastConfigReloadError)
	}
}

func TestShutdownRacingBackgroundWorkerDiscardsGenerationAndClosesPool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cervterm.lua")
	writeBackgroundPNG(t, dir, "wall.png", color.RGBA{R: 255, B: 255, A: 255})
	writeReloadConfig(t, path, `return {config_version=2,background={layers={{kind="image",path="wall.png"}}}}`)
	app := &App{configPath: path, candidateOptions: script.CandidateOptions{}, configWatch: newConfigWatchState(path), lastFBW: 8, lastFBH: 6}
	app.startConfigReloadWorker()
	pool := app.configReloadAsync.resourcePool
	app.discardConfigReloadWorkers()
	deadline := time.Now().Add(3 * time.Second)
	for !pool.isClosed() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !pool.isClosed() || app.configReloadAsync.workers != 0 {
		t.Fatalf("shutdown cleanup: closed=%v workers=%d", pool.isClosed(), app.configReloadAsync.workers)
	}
}
