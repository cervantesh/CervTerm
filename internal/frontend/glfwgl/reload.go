//go:build glfw

package glfwgl

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"reflect"
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

type configWatchState struct {
	path        string
	last        configFileSignature
	initialized bool
	nextPoll    time.Time
	dirtySince  time.Time
}

func newConfigWatchState(path string) configWatchState {
	w := configWatchState{path: path}
	w.acknowledge()
	return w
}

func fileSignature(path string) (configFileSignature, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return configFileSignature{}, false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return configFileSignature{}, false
	}
	return configFileSignature{modTime: info.ModTime().UnixNano(), size: info.Size(), hash: sha256.Sum256(content)}, true
}

func (w *configWatchState) acknowledge() {
	if w.path == "" {
		return
	}
	if sig, ok := fileSignature(w.path); ok {
		w.last = sig
		w.initialized = true
	}
	w.dirtySince = time.Time{}
}

// poll reports one debounced source-file change. It watches only the selected
// source path, so compiling a .tl file cannot recursively trigger on its .lua
// sibling.
func (w *configWatchState) poll(now time.Time) bool {
	if w.path == "" || now.Before(w.nextPoll) {
		return false
	}
	w.nextPoll = now.Add(configPollInterval)
	sig, ok := fileSignature(w.path)
	if !ok {
		return false
	}
	if !w.initialized {
		w.last, w.initialized = sig, true
		return false
	}
	if sig != w.last {
		w.last = sig
		w.dirtySince = now
		return false
	}
	if !w.dirtySince.IsZero() && now.Sub(w.dirtySince) >= configReloadDebounce {
		w.dirtySince = time.Time{}
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

func (a *App) reloadConfig() error {
	if a.configPath == "" {
		return fmt.Errorf("no config source is active")
	}
	sourceBefore, _ := fileSignature(a.configPath)
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
	if candidate != nil {
		if _, err := candidate.PublishTeal(a.tealPublicationOptions); err != nil {
			return err
		}
		candidateRT = activation.Commit()
	}
	restartRequired := restartRequiredChanges(a.cfg, loaded.Config)
	oldBundle, oldRT := a.scriptBundle, a.scriptRT
	a.commitLiveConfig(prepared)
	a.scriptBundle = candidate
	a.scriptRT = candidateRT
	candidate, candidateRT = nil, nil
	if legacyTransition != nil {
		legacyTransition.Commit()
		legacyTransition = nil
	}
	a.status.seq = -1
	a.overlays.seq = -1
	a.configWatch.acknowledge()
	if sourceAfter, ok := fileSignature(a.configPath); ok && sourceAfter != sourceBefore {
		// The source changed while the candidate was being evaluated. Commit the
		// valid snapshot and queue the newer edit rather than swallowing it.
		a.reloadPending = true
	}
	if oldBundle != nil {
		oldBundle.Close()
	} else if oldRT != nil {
		oldRT.Close()
	}
	if restartRequired {
		log.Printf("config reloaded; non-live fields changed and require restart")
		a.Notify("config reloaded; some changes require restart")
	} else {
		a.Notify("config reloaded")
	}
	return nil
}

func restartRequiredChanges(current, candidate config.Config) bool {
	live := current
	live.Window.Opacity = candidate.Window.Opacity
	live.Window.Blur = candidate.Window.Blur
	live.Colors = candidate.Colors
	live.Scrolling = candidate.Scrolling
	live.Scrollbar = candidate.Scrollbar
	return !reflect.DeepEqual(live, candidate)
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
	prepared, err := a.prepareLiveConfig(next)
	if err != nil {
		return err
	}
	defer prepared.Close()
	a.commitLiveConfig(prepared)
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

func (a *App) RuntimeConfig() config.Config { return a.cfg }

func (a *App) ApplyRuntimeConfig(next config.Config) error { return a.applyLiveConfig(next) }

func (a *App) RequestConfigReload() bool { return a.requestConfigReload() }
