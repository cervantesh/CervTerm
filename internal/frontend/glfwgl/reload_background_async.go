//go:build glfw

package glfwgl

import (
	"path/filepath"
	"reflect"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/script"
)

type configReloadAsyncState struct {
	lastNoticeAt           time.Time
	generation             uint64
	workers                int
	results                chan *PreparedAppearanceGeneration
	prepared               *backgroundReloadPrepared
	activeGPUBytes         uint64
	resourcePool           *backgroundResourcePool
	resizeWorkers          int
	activeBackgroundDPI    float64
	requestedBackgroundDPI float64
}

type appearanceGenerationState uint8

const (
	appearanceMissing appearanceGenerationState = iota
	appearanceReading
	appearanceDecoding
	appearancePreparedCPU
	appearancePreparedGPU
	appearanceActive
	appearanceClosed
)

type PreparedAppearanceGeneration struct {
	configReloadCPUResult
	state  appearanceGenerationState
	closed bool
}

type configReloadCPUResult struct {
	generation   uint64
	loaded       script.VersionedSource
	cpu          *preparedBackgroundCPU
	expectations []config.SourceWatchExpectation
	watchBefore  configWatchSnapshot
	err          error
}

func (g *PreparedAppearanceGeneration) Close() {
	if g == nil || g.closed || g.state == appearanceActive {
		return
	}
	g.closed = true
	closeVersionedSource(&g.loaded)
	if g.cpu != nil {
		g.cpu.Close()
		g.cpu = nil
	}
	g.state = appearanceClosed
}

func (g *PreparedAppearanceGeneration) transferComplete() {
	if g == nil {
		return
	}
	g.loaded = script.VersionedSource{}
	if g.cpu != nil {
		g.cpu.Close()
		g.cpu = nil
	}
	g.state = appearanceActive
}

type backgroundReloadPrepared struct {
	config config.Config
	cpu    *preparedBackgroundCPU
}

func (a *App) startConfigReloadWorker() {
	if a.configReloadAsync.results == nil {
		a.configReloadAsync.results = make(chan *PreparedAppearanceGeneration, 4)
	}
	a.configReloadAsync.generation++
	generation := a.configReloadAsync.generation
	a.configReloadAsync.workers++
	path := a.configPath
	options := a.candidateOptions.Clone()
	width, height := a.lastFBW, a.lastFBH
	if a.window != nil {
		width, height = a.window.GetFramebufferSize()
	}
	results := a.configReloadAsync.results
	watchBefore := a.configWatch.snapshot()
	pool := a.ensureBackgroundResourcePool()
	dpi := effectiveDPI(a.contentScaleX, a.contentScaleY)
	go func() {
		result := &PreparedAppearanceGeneration{configReloadCPUResult: configReloadCPUResult{generation: generation, watchBefore: watchBefore}, state: appearanceReading}
		loaded, err := script.LoadVersioned(path, config.Defaults(), options)
		if err != nil {
			result.err = err
			result.expectations = script.FailedWatchExpectations(err)
			results <- result
			a.wakeMainLoop()
			return
		}
		result.loaded = loaded
		provenance := []config.ProvenanceRecord(nil)
		if loaded.Candidate != nil {
			provenance = loaded.Candidate.Provenance()
		}
		baseDir := backgroundLayerBase(provenance, path)
		attempts := backgroundAttemptPaths(loaded.Config, baseDir)
		result.expectations = watchExpectations(append(append([]string(nil), loaded.WatchPaths...), attempts...))
		if len(loaded.Config.Background.Layers) > 0 {
			result.state = appearanceDecoding
			result.cpu, result.err = pool.prepare(loaded.Config, baseDir, width, height, dpi)
		}
		if result.err == nil {
			result.state = appearancePreparedCPU
		}
		results <- result
		a.wakeMainLoop()
	}()
}

func backgroundAttemptPaths(cfg config.Config, baseDir string) []string {
	paths := make([]string, 0, len(cfg.Background.Layers))
	for _, layer := range cfg.Background.Layers {
		if layer.Kind != "image" {
			continue
		}
		path := layer.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		if absolute, err := filepath.Abs(path); err == nil {
			paths = append(paths, filepath.Clean(absolute))
		}
	}
	return paths
}

func (a *App) applyConfigReloadWorkerResults() {
	for a.configReloadAsync.results != nil {
		select {
		case result := <-a.configReloadAsync.results:
			if a.configReloadAsync.workers > 0 {
				a.configReloadAsync.workers--
			}
			if result.generation != a.configReloadAsync.generation {
				result.Close()
				continue
			}
			if result.err != nil {
				result.Close()
				a.lastConfigReloadError = result.err.Error()
				a.acknowledgeConfigReloadFailure(result.watchBefore, result.expectations)
				a.reportConfigReloadFailure(result.err, time.Now())
				continue
			}
			a.configReloadAsync.prepared = &backgroundReloadPrepared{config: result.loaded.Config, cpu: result.cpu}
			err := a.activateLoadedConfig(result.loaded, result.watchBefore, result.expectations, result)
			a.configReloadAsync.prepared = nil
			result.loaded = script.VersionedSource{}
			if err != nil {
				result.Close()
				a.reportConfigReloadFailure(err, time.Now())
			} else {
				result.transferComplete()
				a.clearConfigReloadFailureNotice()
			}
		default:
			return
		}
	}
}

func (a *App) discardConfigReloadWorkers() {
	configCount := a.configReloadAsync.workers
	resizeCount := a.configReloadAsync.resizeWorkers
	pool := a.configReloadAsync.resourcePool
	if configCount == 0 && resizeCount == 0 {
		if pool != nil {
			_ = pool.close()
		}
		return
	}
	a.configReloadAsync.generation++
	a.backgroundGeneration++
	a.configReloadAsync.workers = 0
	a.configReloadAsync.resizeWorkers = 0
	configResults := a.configReloadAsync.results
	resizeResults := a.backgroundResizeResults
	go func() {
		for index := 0; index < configCount; index++ {
			result := <-configResults
			result.Close()
		}
		for index := 0; index < resizeCount; index++ {
			result := <-resizeResults
			if result.cpu != nil {
				result.cpu.Close()
			}
		}
		if pool != nil {
			_ = pool.close()
		}
	}()
}

func closeVersionedSource(loaded *script.VersionedSource) {
	if loaded == nil {
		return
	}
	if loaded.Candidate != nil {
		loaded.Candidate.Close()
		loaded.Candidate = nil
	} else if loaded.Runtime != nil {
		loaded.Runtime.Close()
		loaded.Runtime = nil
	}
	if loaded.LegacyTransition != nil {
		_ = loaded.LegacyTransition.Rollback()
		loaded.LegacyTransition = nil
	}
}

func (a *App) consumePreparedBackground(next config.Config) *preparedBackgroundCPU {
	prepared := a.configReloadAsync.prepared
	if prepared == nil || prepared.cpu == nil {
		return nil
	}
	if !reflect.DeepEqual(prepared.config.Background, next.Background) || prepared.config.Colors.Background != next.Colors.Background || prepared.config.Window.BackgroundOpacity != next.Window.BackgroundOpacity {
		return nil
	}
	return prepared.cpu
}

func (a *App) layeredBackgroundCPU(next config.Config, provenance []config.ProvenanceRecord) (*preparedBackgroundCPU, error) {
	if prepared := a.consumePreparedBackground(next); prepared != nil {
		return prepared, nil
	}
	width, height := a.lastFBW, a.lastFBH
	if a.window != nil {
		width, height = a.window.GetFramebufferSize()
	}
	result := make(chan struct {
		cpu *preparedBackgroundCPU
		err error
	}, 1)
	baseDir := backgroundLayerBase(provenance, a.configPath)
	pool := a.ensureBackgroundResourcePool()
	dpi := effectiveDPI(a.contentScaleX, a.contentScaleY)
	go func() {
		cpu, err := pool.prepare(next.Clone(), baseDir, width, height, dpi)
		result <- struct {
			cpu *preparedBackgroundCPU
			err error
		}{cpu: cpu, err: err}
	}()
	prepared := <-result
	return prepared.cpu, prepared.err
}
