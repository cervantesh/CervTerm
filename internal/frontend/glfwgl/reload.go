//go:build glfw

package glfwgl

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/fontglyph"
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
	if message == a.lastReloadNoticeError && !a.lastReloadNoticeAt.IsZero() && now.Sub(a.lastReloadNoticeAt) < configReloadFailureNoticeInterval {
		return false
	}
	a.lastReloadNoticeError = message
	a.lastReloadNoticeAt = now
	log.Printf("config reload failed: %v", err)
	a.Notify("config reload failed: " + message)
	return true
}

func (a *App) clearConfigReloadFailureNotice() {
	a.lastReloadNoticeError = ""
	a.lastReloadNoticeAt = time.Time{}
}

func (a *App) acknowledgeConfigReloadFailure(before configWatchSnapshot, expectations []config.SourceWatchExpectation) {
	changedDuringAttempt := configWatchSnapshotsDiffer(before, a.configWatch.snapshot())
	failedSetChanged := a.configWatch.acknowledgeFailure(expectations)
	if changedDuringAttempt || failedSetChanged {
		a.reloadPending = true
	}
}

func (a *App) applyPendingConfigReload() {
	if !a.reloadPending {
		return
	}
	a.reloadPending = false
	if err := a.reloadConfig(); err != nil {
		a.reportConfigReloadFailure(err, time.Now())
		return
	}
	a.clearConfigReloadFailureNotice()
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
	failureExpectations := watchExpectations(loaded.WatchPaths)
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
	prepared, err := a.prepareLiveConfig(resolvedDesired)
	if err != nil {
		return err
	}
	defer prepared.Close()
	candidateFilesChanged := watchHashesChanged(loaded.WatchHashes)
	if candidate != nil {
		if _, err := candidate.PublishTeal(a.tealPublicationOptions); err != nil {
			return err
		}
		candidateRT = activation.Commit()
	}
	watchAfterEvaluation := a.configWatch.snapshot()
	changedDuringEvaluation := configWatchSnapshotsDiffer(watchBefore, watchAfterEvaluation)
	oldBundle, oldRT := a.scriptBundle, a.scriptRT
	a.commitLiveConfig(prepared)
	a.composedCfg = loaded.Config.Clone()
	a.composedProvenance = append([]config.ProvenanceRecord(nil), candidateProvenance...)
	a.desiredCfg = resolvedDesired.Clone()
	a.runtimeOverrideRecords = append([]config.RuntimeOverrideRecord(nil), runtimeRecords...)
	a.pendingConfig = config.PendingConfigChanges(a.desiredCfg, a.cfg)
	a.scriptBundle = candidate
	a.scriptRT = candidateRT
	candidate, candidateRT = nil, nil
	if legacyTransition != nil {
		legacyTransition.Commit()
		legacyTransition = nil
	}
	a.status.seq = -1
	a.overlays.seq = -1
	a.configWatch.acknowledgeSuccess(loaded.WatchPaths)
	changedBeforeAcknowledgement := a.configWatch.changedSince(watchAfterEvaluation)
	candidateFilesChanged = candidateFilesChanged || watchHashesChanged(loaded.WatchHashes)
	if changedDuringEvaluation || changedBeforeAcknowledgement || candidateFilesChanged {
		// An active source/include/module changed during evaluation or in the
		// commit window. Queue a newer generation rather than acknowledging it away.
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

// preparedLiveConfig owns all resources created by the fallible preparation
// phase until commit transfers them to the App or Close aborts them.
type preparedLiveConfig struct {
	next             config.Config
	preparedContexts map[atlasFontKey]*atlasFontContext
	rasterChanged    bool
	committed        bool
}

func (p *preparedLiveConfig) Close() {
	if p == nil || p.committed {
		return
	}
	closePreparedRasterContexts(p.preparedContexts)
	p.preparedContexts = nil
}

// prepareLiveConfig validates and constructs every fallible frontend resource
// without mutating active application state.
func (a *App) prepareLiveConfig(next config.Config) (*preparedLiveConfig, error) {
	if err := next.Validate(); err != nil {
		return nil, err
	}
	oldRaster := a.effectiveTextRaster()
	liveNext := a.cfg
	liveNext.Colors = next.Colors
	newRaster := effectiveTextRasterFor(liveNext)
	prepared := &preparedLiveConfig{next: next, rasterChanged: a.atlas != nil && oldRaster != newRaster}
	if prepared.rasterChanged {
		contexts, err := a.prepareRasterContexts(newRaster)
		if err != nil {
			return nil, fmt.Errorf("prepare text raster: %w", err)
		}
		prepared.preparedContexts = contexts
	}
	return prepared, nil
}

// commitLiveConfig is the mechanically infallible main-thread mutation phase.
// The caller must have completed every fallible operation before invoking it.
func (a *App) commitLiveConfig(prepared *preparedLiveConfig) {
	next := prepared.next
	oldScrollbar := a.cfg.Scrollbar
	a.mux.SetScrollbackCapacity(next.Scrolling.History)
	a.mux.SetHideCursorWhenScrolled(next.Scrolling.HideCursorWhenScrolled)
	a.syncFocusedProjection()
	a.cfg.Window.Opacity = next.Window.Opacity
	a.cfg.Window.Blur = next.Window.Blur
	a.cfg.Colors = next.Colors
	a.mux.SetPaletteBase(configuredPaletteBase(a.cfg.Colors))
	a.cfg.Scrolling = next.Scrolling
	a.cfg.Scrollbar = next.Scrollbar
	a.cfg.Cursor = next.Cursor
	a.applyWindowAppearance()
	if prepared.rasterChanged {
		a.installPreparedRasterContexts(prepared.preparedContexts)
	}
	prepared.preparedContexts = nil
	prepared.committed = true
	if !a.cfg.Scrollbar.Enabled {
		a.scrollbar = scrollbarState{}
	} else {
		a.scrollbar.lastActivity = time.Now()
	}
	if a.window != nil && (oldScrollbar.Enabled != a.cfg.Scrollbar.Enabled || oldScrollbar.ReservedWidthPX != a.cfg.Scrollbar.ReservedWidthPX) {
		a.resizeToWindow()
	}
	a.damage.valid = false
	a.requestRedraw()
}

// applyLiveConfig preserves the existing runtime-setter contract while sharing
// the prepare/commit seam used by atomic configuration activation.
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

func (a *App) prepareRasterContexts(textRaster string) (map[atlasFontKey]*atlasFontContext, error) {
	prepared := make(map[atlasFontKey]*atlasFontContext)
	sizes := []float64{a.cfg.Font.Size}
	if a.mux != nil && len(a.mux.PaneIDs()) > 0 {
		sizes = sizes[:0]
		for _, id := range a.mux.PaneIDs() {
			size := a.cfg.Font.Size
			if state := a.paneUI[id]; state != nil && state.font.fontSize > 0 {
				size = state.font.fontSize
			}
			sizes = append(sizes, size)
		}
	}
	for _, size := range sizes {
		spec := fontglyph.Spec{Family: a.cfg.Font.Family, Size: size, DPI: effectiveDPI(a.contentScaleX, a.contentScaleY), TextRaster: textRaster}
		model := a.atlas.modelForSpec(spec)
		key, err := makeAtlasFontKeyWithModel(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken, model)
		if err != nil {
			closePreparedRasterContexts(prepared)
			return nil, err
		}
		if _, ok := a.atlas.contexts[key]; ok {
			continue
		}
		if _, ok := prepared[key]; ok {
			continue
		}
		ctx, err := makeAtlasFontContextWithModel(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken, model, a.atlas.backendFactory)
		if err != nil {
			closePreparedRasterContexts(prepared)
			return nil, err
		}
		prepared[key] = ctx
	}
	return prepared, nil
}

func closePreparedRasterContexts(prepared map[atlasFontKey]*atlasFontContext) {
	closed := make([]fontglyph.Backend, 0, len(prepared))
	for _, ctx := range prepared {
		duplicate := false
		for _, backend := range closed {
			if sameAtlasBackend(ctx.backend, backend) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			ctx.backend.Close()
			closed = append(closed, ctx.backend)
		}
	}
}

func (a *App) installPreparedRasterContexts(prepared map[atlasFontKey]*atlasFontContext) {
	for key, ctx := range prepared {
		a.atlas.contexts[key] = ctx
	}
	if a.mux == nil || len(a.mux.PaneIDs()) == 0 {
		cellW, cellH, _, ok := a.atlas.useSpec(a.fontSpec(a.cfg.Font.Size, a.contentScaleX, a.contentScaleY), a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
		if ok {
			a.cellW, a.cellH = float32(cellW), float32(cellH)
			a.ligaturesActive = a.cfg.Font.Ligatures && a.atlas.supportsLigatures()
		}
		return
	}
	for _, id := range a.mux.PaneIDs() {
		state := a.ensurePaneUI(id)
		gridChanged, applied := a.applyPaneFontVisual(id, state.font.fontSize, a.contentScaleX, a.contentScaleY)
		state.font.ptyDirty = state.font.ptyDirty || (applied && gridChanged)
	}
	now := time.Now()
	for _, id := range a.mux.PaneIDs() {
		state := a.ensurePaneUI(id)
		if !state.font.ptyDirty {
			continue
		}
		if a.applyPanePTYResize(id) {
			state.font.ptyDirty = false
			continue
		}
		a.schedulePanePTYResizeRetry(id, now)
	}
	a.restoreFocusedFontProjection()
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
