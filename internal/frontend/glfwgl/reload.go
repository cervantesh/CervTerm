//go:build glfw

package glfwgl

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/fontglyph"
	"cervterm/internal/script"
)

const (
	configPollInterval   = 250 * time.Millisecond
	configReloadDebounce = 200 * time.Millisecond
)

type configFileSignature struct {
	modTime int64
	size    int64
	hash    [sha256.Size]byte
}

type configFileObservation struct {
	signature configFileSignature
	exists    bool
}

type configWatchSnapshot struct {
	generation uint64
	files      map[string]configFileObservation
}

type configWatchState struct {
	paths       []string
	baseline    map[string]configFileObservation
	observed    map[string]configFileObservation
	initialized bool
	generation  uint64
	nextPoll    time.Time
	dirtySince  time.Time
}

func newConfigWatchState(paths ...string) configWatchState {
	w := configWatchState{}
	w.acknowledge(paths)
	return w
}

func fileObservation(path string) configFileObservation {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return configFileObservation{}
	}
	hash, err := config.FileSourceWatchHash(path)
	if err != nil {
		return configFileObservation{}
	}
	return configFileObservation{exists: true, signature: configFileSignature{modTime: info.ModTime().UnixNano(), size: info.Size(), hash: hash}}
}

func fileSignature(path string) (configFileSignature, bool) {
	observation := fileObservation(path)
	return observation.signature, observation.exists
}

func normalizeWatchPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func observeWatchPaths(paths []string) map[string]configFileObservation {
	observed := make(map[string]configFileObservation, len(paths))
	for _, path := range paths {
		observed[path] = fileObservation(path)
	}
	return observed
}

func (w *configWatchState) acknowledge(paths []string) {
	w.paths = normalizeWatchPaths(paths)
	w.baseline = observeWatchPaths(w.paths)
	w.observed = cloneWatchObservations(w.baseline)
	w.initialized = len(w.paths) > 0
	w.generation++
	w.dirtySince = time.Time{}
}

func cloneWatchObservations(source map[string]configFileObservation) map[string]configFileObservation {
	clone := make(map[string]configFileObservation, len(source))
	for path, observation := range source {
		clone[path] = observation
	}
	return clone
}

func (w *configWatchState) snapshot() configWatchSnapshot {
	return configWatchSnapshot{generation: w.generation, files: observeWatchPaths(w.paths)}
}

func (w *configWatchState) changedSince(snapshot configWatchSnapshot) bool {
	return !reflect.DeepEqual(snapshot.files, observeWatchPaths(mapsKeys(snapshot.files)))
}

func watchHashesChanged(hashes map[string][32]byte) bool {
	for path, expected := range hashes {
		observation := fileObservation(path)
		if !observation.exists || observation.signature.hash != expected {
			return true
		}
	}
	return false
}

func configWatchSnapshotsDiffer(left, right configWatchSnapshot) bool {
	return left.generation != right.generation || !reflect.DeepEqual(left.files, right.files)
}

func mapsKeys(values map[string]configFileObservation) []string {
	paths := make([]string, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// poll reports one debounced change across the complete active source graph.
// Missing files are observations too, so deletion/rename triggers a reload.
func (w *configWatchState) poll(now time.Time) bool {
	if len(w.paths) == 0 || now.Before(w.nextPoll) {
		return false
	}
	w.nextPoll = now.Add(configPollInterval)
	current := observeWatchPaths(w.paths)
	if !w.initialized {
		w.baseline, w.observed, w.initialized = current, cloneWatchObservations(current), true
		return false
	}
	if !reflect.DeepEqual(current, w.observed) {
		w.observed = current
		if reflect.DeepEqual(current, w.baseline) {
			w.dirtySince = time.Time{}
		} else {
			w.dirtySince = now
		}
		return false
	}
	if !w.dirtySince.IsZero() && now.Sub(w.dirtySince) >= configReloadDebounce {
		w.baseline = cloneWatchObservations(current)
		w.dirtySince = time.Time{}
		w.generation++
		return true
	}
	return false
}

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

func (a *App) applyPendingConfigReload() {
	if !a.reloadPending {
		return
	}
	a.reloadPending = false
	if err := a.reloadConfig(); err != nil {
		log.Printf("config reload failed: %v", err)
		a.Notify("config reload failed: " + err.Error())
	}
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
	loaded, err := script.LoadVersioned(a.configPath, config.Defaults(), script.CandidateOptions{})
	if err != nil {
		return err
	}
	candidate, candidateRT := loaded.Candidate, loaded.Runtime
	legacyTransition := loaded.LegacyTransition
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
	var activation *script.CandidateActivation
	if candidate != nil {
		activation, err = candidate.PrepareActivation()
		if err != nil {
			return err
		}
	}
	prepared, err := a.prepareLiveConfig(loaded.Config)
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
	a.desiredCfg = loaded.Config.Clone()
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
	a.configWatch.acknowledge(loaded.WatchPaths)
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
	if a.configStateInitialized {
		return
	}
	a.desiredCfg = a.cfg.Clone()
	a.pendingConfig = config.PendingConfigChanges(a.desiredCfg, a.cfg)
	a.configStateInitialized = true
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
	prepared, err := a.prepareLiveConfig(next)
	if err != nil {
		return err
	}
	defer prepared.Close()
	a.commitLiveConfig(prepared)
	a.desiredCfg = config.MergeLiveConfig(a.desiredCfg, next).Clone()
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
		key := newAtlasFontKey(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
		if _, ok := a.atlas.contexts[key]; ok {
			continue
		}
		if _, ok := prepared[key]; ok {
			continue
		}
		ctx, err := makeAtlasFontContext(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken, a.atlas.backendFactory)
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

func (a *App) ApplyRuntimeConfig(next config.Config) error { return a.applyLiveConfig(next) }

func (a *App) RequestConfigReload() bool { return a.requestConfigReload() }
