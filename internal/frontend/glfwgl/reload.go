//go:build glfw

package glfwgl

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/frontend/gpu"
	"cervterm/internal/script"
)

func (a *App) requestConfigReload() bool {
	if a.configPath == "" {
		return false
	}
	a.reloadPending = true
	return true
}

func (a *App) pollConfigReload(now time.Time) {
	if a.configWatch.poll(now) {
		a.reloadPending = true
	}
}

const configReloadFailureNoticeInterval = 30 * time.Second

func (a *App) reportConfigReloadFailure(err error, now time.Time) bool {
	message := err.Error()
	if message == a.lastReloadNoticeError && !a.configReloadAsync.lastNoticeAt.IsZero() && now.Sub(a.configReloadAsync.lastNoticeAt) < configReloadFailureNoticeInterval {
		return false
	}
	a.lastReloadNoticeError = message
	a.configReloadAsync.lastNoticeAt = now
	log.Printf("config reload failed: %v", err)
	a.Notify("config reload failed: " + message)
	return true
}

func (a *App) clearConfigReloadFailureNotice() {
	a.lastReloadNoticeError = ""
	a.configReloadAsync.lastNoticeAt = time.Time{}
}

func (a *App) acknowledgeConfigReloadFailure(before configWatchSnapshot, expectations []config.SourceWatchExpectation) {
	changedDuringAttempt := configWatchSnapshotsDiffer(before, a.configWatch.snapshot())
	failedSetChanged := a.configWatch.acknowledgeFailure(expectations)
	if changedDuringAttempt || failedSetChanged {
		a.reloadPending = true
	}
}

func (a *App) applyPendingConfigReload() {
	a.applyConfigReloadWorkerResults()
	if !a.reloadPending {
		return
	}
	if a.configReloadAsync.workers >= 2 {
		return
	}
	a.reloadPending = false
	if a.configPath == "" {
		a.reportConfigReloadFailure(fmt.Errorf("no config source is active"), time.Now())
		return
	}
	a.startConfigReloadWorker()
}

func (a *App) reloadConfig() (resultErr error) {
	a.ensureConfigState()
	defer func() {
		if resultErr != nil {
			a.lastConfigReloadError = resultErr.Error()
		} else {
			a.lastConfigReloadError = ""
		}
	}()
	if a.configPath == "" {
		return fmt.Errorf("no config source is active")
	}
	watchBefore := a.configWatch.snapshot()
	loaded, err := script.LoadVersioned(a.configPath, config.Defaults(), a.candidateOptions.Clone())
	if err != nil {
		a.acknowledgeConfigReloadFailure(watchBefore, script.FailedWatchExpectations(err))
		return err
	}
	return a.activateLoadedConfig(loaded, watchBefore, nil, nil)
}

func (a *App) activateLoadedConfig(loaded script.VersionedSource, watchBefore configWatchSnapshot, workerExpectations []config.SourceWatchExpectation, generation *PreparedAppearanceGeneration) (resultErr error) {
	a.ensureConfigState()
	defer func() {
		if resultErr != nil {
			a.lastConfigReloadError = resultErr.Error()
		} else {
			a.lastConfigReloadError = ""
		}
	}()
	failureExpectations := watchExpectations(loaded.WatchPaths)
	if len(workerExpectations) > 0 {
		failureExpectations = workerExpectations
	}
	defer func() {
		if resultErr == nil {
			return
		}
		a.acknowledgeConfigReloadFailure(watchBefore, failureExpectations)
	}()
	candidate, candidateRT := loaded.Candidate, loaded.Runtime
	legacyTransition := loaded.LegacyTransition
	var candidateProvenance []config.ProvenanceRecord
	if candidate != nil {
		candidateProvenance = candidate.Provenance()
	}
	defer func() {
		if candidate != nil {
			candidate.Close()
		} else if candidateRT != nil {
			candidateRT.Close()
		}
	}()
	defer func() {
		if legacyTransition != nil {
			if err := legacyTransition.Rollback(); err != nil {
				log.Printf("rollback legacy Teal transition: %v", err)
			}
		}
	}()
	if loaded.AuthoredVersion != 2 && a.candidateOptions.RequiresVersion2() {
		return fmt.Errorf("--environment, --profile, and --config-override require config_version=2")
	}
	resolvedDesired, runtimeRecords, err := a.runtimeScopes.Apply(a.configScope, loaded.Config)
	if err != nil {
		return fmt.Errorf("reapply runtime config scope %s: %w", a.configScope, err)
	}
	var activation *script.CandidateActivation
	if candidate != nil {
		activation, err = candidate.PrepareActivation()
		if err != nil {
			return err
		}
	}
	prepared, err := a.prepareLiveConfigWithProvenance(resolvedDesired, candidateProvenance)
	if err != nil {
		return err
	}
	if generation != nil && prepared.backgroundChanged {
		generation.state = appearancePreparedGPU
	}
	backgroundWatchPaths, backgroundWatchHashes := prepared.backgroundWatchPaths, prepared.backgroundWatchHashes
	if len(backgroundWatchPaths) == 0 && a.configReloadAsync.prepared != nil && a.configReloadAsync.prepared.cpu != nil {
		backgroundWatchPaths = a.configReloadAsync.prepared.cpu.watchPaths
		backgroundWatchHashes = a.configReloadAsync.prepared.cpu.watchHashes
	}
	if len(backgroundWatchPaths) > 0 {
		loaded.WatchPaths = append(loaded.WatchPaths, backgroundWatchPaths...)
		if loaded.WatchHashes == nil {
			loaded.WatchHashes = make(map[string][32]byte)
		}
		for path, hash := range backgroundWatchHashes {
			loaded.WatchHashes[path] = hash
		}
		failureExpectations = watchExpectations(loaded.WatchPaths)
	}
	defer prepared.Close()
	watchAfterEvaluation := a.configWatch.snapshot()
	changedDuringEvaluation := configWatchSnapshotsDiffer(watchBefore, watchAfterEvaluation)
	candidateFilesChanged := watchHashesChanged(loaded.WatchHashes)
	if changedDuringEvaluation || candidateFilesChanged {
		a.reloadPending = true
		return fmt.Errorf("config sources changed while preparing reload; queued a newer generation")
	}
	if candidate != nil {
		if _, err := candidate.PublishTeal(a.tealPublicationOptions); err != nil {
			return err
		}
		candidateRT = activation.Commit()
	}
	a.keyTable.cancel()
	oldBundle, oldRT := a.scriptBundle, a.scriptRT
	a.commitLiveConfig(prepared)
	a.composedCfg = loaded.Config.Clone()
	a.composedProvenance = append([]config.ProvenanceRecord(nil), candidateProvenance...)
	a.desiredCfg = resolvedDesired.Clone()
	a.runtimeOverrideRecords = append([]config.RuntimeOverrideRecord(nil), runtimeRecords...)
	a.pendingConfig = config.PendingConfigChanges(a.desiredCfg, a.cfg)
	a.scriptBundle = candidate
	a.installScriptRuntime(candidateRT)
	a.scriptGeneration++
	candidate, candidateRT = nil, nil
	if legacyTransition != nil {
		legacyTransition.Commit()
		legacyTransition = nil
	}
	a.status.seq = -1
	a.overlays.seq = -1
	a.configWatch.acknowledgeSuccess(loaded.WatchPaths)
	changedBeforeAcknowledgement := a.configWatch.changedSince(watchAfterEvaluation)
	if changedBeforeAcknowledgement || watchHashesChanged(loaded.WatchHashes) {
		// A source changed in the non-fallible commit window; queue a newer generation.
		a.reloadPending = true
	}
	if oldBundle != nil {
		oldBundle.Close()
	} else if oldRT != nil {
		oldRT.Close()
	}
	if len(a.pendingConfig) > 0 {
		detail := formatPendingConfigChanges(a.pendingConfig, 3)
		log.Printf("config reloaded; pending scoped changes: %s", detail)
		a.Notify("config reloaded; pending: " + detail)
	} else {
		a.Notify("config reloaded")
	}
	return nil
}

func (a *App) ensureConfigState() {
	if !a.configStateInitialized {
		a.desiredCfg = a.cfg.Clone()
		a.composedCfg = a.cfg.Clone()
		a.pendingConfig = config.PendingConfigChanges(a.desiredCfg, a.cfg)
		a.configStateInitialized = true
	}
	if reflect.DeepEqual(a.composedCfg, config.Config{}) {
		a.composedCfg = a.cfg.Clone()
	}
	if !a.configScope.Valid() {
		a.configScope = a.runtimeScopes.NewScope()
	}
}

func formatPendingConfigChanges(changes []config.ConfigChange, limit int) string {
	if limit <= 0 || limit > len(changes) {
		limit = len(changes)
	}
	parts := make([]string, 0, limit+1)
	for _, change := range changes[:limit] {
		parts = append(parts, fmt.Sprintf("%s (%s)", change.Path, change.Scope))
	}
	if remaining := len(changes) - limit; remaining > 0 {
		parts = append(parts, fmt.Sprintf("+%d more", remaining))
	}
	return strings.Join(parts, ", ")
}

func (a *App) prepareLiveConfigWithProvenance(next config.Config, provenance []config.ProvenanceRecord) (*preparedLiveConfig, error) {
	var projectionBase *config.Config
	if a.restoreAppearance != nil {
		base := next.Clone()
		projectionBase = &base
	}
	next = a.configWithRestoreAppearance(next)
	if err := next.Validate(); err != nil {
		return nil, err
	}
	oldRaster := a.effectiveTextRaster()
	liveNext := a.cfg
	liveNext.Colors = next.Colors
	liveNext.Window.TextOpacity = next.Window.TextOpacity
	liveNext.Window.BackgroundOpacity = next.Window.BackgroundOpacity
	newRaster := effectiveTextRasterFor(liveNext)
	backgroundChanged := a.cfg.Colors.Background != next.Colors.Background || a.cfg.Window.BackgroundOpacity != next.Window.BackgroundOpacity || !reflect.DeepEqual(a.cfg.Background.Layers, next.Background.Layers)
	prepared := &preparedLiveConfig{next: next, projectionBase: projectionBase, rasterChanged: a.atlas != nil && oldRaster != newRaster, backgroundChanged: backgroundChanged}
	if prepared.rasterChanged {
		contexts, err := a.prepareRasterContexts(newRaster)
		if err != nil {
			return nil, fmt.Errorf("prepare text raster: %w", err)
		}
		prepared.preparedContexts = contexts
		pins := make(map[atlasFontKey]struct{})
		if a.mux != nil {
			layout, layoutErr := a.mux.Layout()
			if layoutErr != nil {
				closePreparedRasterContexts(contexts)
				return nil, fmt.Errorf("prepare text raster pins: %w", layoutErr)
			}
			pins = a.visibleFontContextKeysForRaster(layout, 0, 0, newRaster)
		}
		if len(pins) == 0 {
			for key := range contexts {
				pins[key] = struct{}{}
				break
			}
		}
		install, installOK := a.atlas.prepareContextInstall(contexts, pins)
		if !installOK {
			closePreparedRasterContexts(contexts)
			return nil, fmt.Errorf("prepare text raster: retained font context limit")
		}
		prepared.contextInstall = install
	}
	if prepared.backgroundChanged {
		var surface gpu.BackgroundSurface
		var err error
		surfaceWidth, surfaceHeight := 0, 0
		candidateBytes := uint64(0)
		candidateDPI := float64(0)
		if len(next.Background.Layers) == 0 {
			pixels := solidBackgroundPixels(liveNext)
			candidateBytes, err = backgroundGPUTransferBytes(a.configReloadAsync.activeGPUBytes, pixels)
			if err == nil {
				surface, err = prepareRGBABackgroundSurface(a.r, pixels)
				if surface == nil {
					candidateBytes = 0
				}
			}
		} else {
			cpuResult, prepareErr := a.layeredBackgroundCPU(next, provenance)
			if prepareErr != nil {
				err = prepareErr
			} else {
				defer cpuResult.Close()
				surfaceWidth, surfaceHeight = cpuResult.surface.Bounds().Dx(), cpuResult.surface.Bounds().Dy()
				candidateDPI = cpuResult.dpi
				candidateBytes, err = backgroundGPUTransferBytes(a.configReloadAsync.activeGPUBytes, cpuResult.surface)
				if err == nil {
					surface, err = prepareRGBABackgroundSurface(a.r, cpuResult.surface)
					if surface == nil && err == nil {
						err = fmt.Errorf("background layers: renderer capability unavailable")
					}
					prepared.backgroundWatchPaths = append([]string(nil), cpuResult.watchPaths...)
					prepared.backgroundWatchHashes = make(map[string][32]byte, len(cpuResult.watchHashes))
					for path, hash := range cpuResult.watchHashes {
						prepared.backgroundWatchHashes[path] = hash
					}
				}
			}
		}
		if err != nil {
			closePreparedRasterContexts(prepared.preparedContexts)
			prepared.preparedContexts = nil
			return nil, err
		}
		prepared.backgroundSurface = surface
		prepared.backgroundWidth, prepared.backgroundHeight = surfaceWidth, surfaceHeight
		prepared.backgroundBytes = candidateBytes
		prepared.backgroundDPI = candidateDPI
	}
	return prepared, nil
}

func (a *App) commitLiveConfig(prepared *preparedLiveConfig) {
	if prepared.rasterChanged {
		a.atlas.commitContextInstall(prepared.contextInstall)
		prepared.preparedContexts = nil
		prepared.contextInstall = nil
	}
	if prepared.backgroundChanged {
		oldSurface := a.backgroundSurface
		a.backgroundSurface = prepared.backgroundSurface
		prepared.backgroundSurface = nil
		a.configReloadAsync.activeGPUBytes = prepared.backgroundBytes
		a.backgroundSurfaceWidth, a.backgroundSurfaceHeight = prepared.backgroundWidth, prepared.backgroundHeight
		a.backgroundRequestedWidth, a.backgroundRequestedHeight = prepared.backgroundWidth, prepared.backgroundHeight
		a.configReloadAsync.activeBackgroundDPI = prepared.backgroundDPI
		a.configReloadAsync.requestedBackgroundDPI = prepared.backgroundDPI
		a.backgroundGeneration++
		if oldSurface != nil {
			_ = oldSurface.Close()
		}
	}
	next := prepared.next
	if prepared.projectionBase != nil {
		base := prepared.projectionBase.Clone()
		a.projectionBaseConfig = &base
	}
	oldScrollbar := a.cfg.Scrollbar
	oldTabBarHeight := a.effectiveTabBarHeight()
	oldTabBarPosition := a.cfg.TabBar.Position
	a.mux.SetScrollbackCapacity(next.Scrolling.History)
	a.mux.SetHideCursorWhenScrolled(next.Scrolling.HideCursorWhenScrolled)
	a.syncFocusedProjection()
	a.cfg.Window.Opacity = next.Window.Opacity
	a.cfg.Window.TextOpacity = next.Window.TextOpacity
	a.cfg.Window.BackgroundOpacity = next.Window.BackgroundOpacity
	a.cfg.Window.Blur = next.Window.Blur
	a.cfg.Colors = next.Colors
	a.cfg.Background = next.Background
	a.cfg.Background.Layers = next.Clone().Background.Layers
	a.mux.SetPaletteBase(configuredPaletteBase(a.cfg.Colors))
	a.cfg.Scrolling = next.Scrolling
	a.cfg.Scrollbar = next.Scrollbar
	a.cfg.TabBar = next.TabBar
	a.cfg.Cursor = next.Cursor
	a.cfg.Render.MaxFPS = next.Render.MaxFPS
	a.applyWindowAppearance()
	if prepared.rasterChanged {
		a.activateInstalledRasterContexts()
	}
	prepared.preparedContexts = nil
	prepared.backgroundSurface = nil
	prepared.committed = true
	if !scrollbarEnabled(a.cfg.Scrollbar) {
		a.scrollbar = scrollbarState{}
	} else {
		a.scrollbar.lastActivity = time.Now()
	}
	geometryChanged := scrollbarGutterWidth(oldScrollbar, a.uiScale) != scrollbarGutterWidth(a.cfg.Scrollbar, a.uiScale) || oldTabBarHeight != a.effectiveTabBarHeight() || oldTabBarPosition != a.cfg.TabBar.Position
	if a.window != nil && geometryChanged {
		a.resizeToWindow()
	}
	a.damage.valid = false
	a.requestRedraw()
}

func (a *App) applyLiveConfig(next config.Config) error {
	a.ensureConfigState()
	transaction, err := a.runtimeScopes.ProposeConfig(a.configScope, a.composedCfg, a.cfg, next)
	if err != nil {
		return err
	}
	return a.applyRuntimeTransaction(transaction)
}

func (a *App) applyRuntimeTransaction(transaction *config.RuntimePatchTransaction) error {
	desired := transaction.Desired()
	prepared, err := a.prepareLiveConfig(desired)
	if err != nil {
		return err
	}
	defer prepared.Close()
	a.commitLiveConfig(prepared)
	transaction.Commit()
	a.desiredCfg = desired.Clone()
	a.runtimeOverrideRecords = transaction.Records()
	a.pendingConfig = config.PendingConfigChanges(a.desiredCfg, a.cfg)
	return nil
}

func (a *App) RuntimeConfig() config.Config { return a.cfg.Clone() }

func (a *App) DesiredConfig() config.Config {
	a.ensureConfigState()
	return a.desiredCfg.Clone()
}

func (a *App) EffectiveConfig() config.Config { return a.cfg.Clone() }

func (a *App) PendingConfigChanges() []config.ConfigChange {
	a.ensureConfigState()
	return append([]config.ConfigChange(nil), a.pendingConfig...)
}

func (a *App) LastConfigReloadError() string { return a.lastConfigReloadError }

func (a *App) ConfigScopeID() config.ConfigScopeID {
	a.ensureConfigState()
	return a.configScope
}

func (a *App) RuntimeConfigOverrides() []config.RuntimeOverrideRecord {
	a.ensureConfigState()
	return append([]config.RuntimeOverrideRecord(nil), a.runtimeOverrideRecords...)
}

func (a *App) RuntimeConfigProvenance() []config.ProvenanceRecord {
	a.ensureConfigState()
	return config.RuntimeOverrideProvenance(a.composedProvenance, a.runtimeOverrideRecords)
}

func (a *App) ClearRuntimeConfigOverrides(paths ...string) error {
	a.ensureConfigState()
	transaction, err := a.runtimeScopes.ProposeClear(a.configScope, a.composedCfg, paths...)
	if err != nil {
		return err
	}
	return a.applyRuntimeTransaction(transaction)
}

func (a *App) ApplyRuntimeConfig(next config.Config) error { return a.applyLiveConfig(next) }

func (a *App) RequestConfigReload() bool { return a.requestConfigReload() }
