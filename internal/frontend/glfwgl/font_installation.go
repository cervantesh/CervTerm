//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"cervterm/internal/config"
	"cervterm/internal/fontglyph"
	"cervterm/internal/frontend/gpu"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"
)

var safeFontsMode atomic.Bool

// SetSafeFontsMode selects safe fonts for the next frontend startup. The mode is
// consumed when Run constructs its App, so library/test callers cannot leak it
// into a later independent run; the active App retains its captured value.
func SetSafeFontsMode(enabled bool) { safeFontsMode.Store(enabled) }

func effectiveStartupConfig(authored config.Config, safe bool) config.Config {
	active := authored.Clone()
	if safe {
		active.Font.Family = "Go Mono"
	}
	return active
}

func Run() error {
	return RunWithConfig(config.Defaults())
}

func RunWithConfig(cfg config.Config) error {
	return RunWithOptions(cfg, nil)
}

func RunWithOptions(cfg config.Config, rt *script.Runtime) error {
	return RunWithSource(cfg, rt, "")
}

func RunWithSource(cfg config.Config, rt *script.Runtime, sourcePath string) error {
	return runWithSource(cfg, rt, nil, nil, nil, []string{sourcePath}, nil, sourcePath, script.CandidateOptions{})
}

func (a *App) initMux() {
	historyCapacity := a.cfg.Scrolling.History
	hideCursorWhenScrolled := a.cfg.Scrolling.HideCursorWhenScrolled
	a.mux = termmux.New(nil, termmux.Options{
		ScrollbackCapacity:     &historyCapacity,
		HideCursorWhenScrolled: &hideCursorWhenScrolled,
		Wake:                   a.wakeMainLoop,
		SetClipboard: func(_ termmux.PaneID, text string) {
			if a.window != nil && a.cfg.Clipboard.OSC52 == "write" {
				a.window.SetClipboardString(text)
			}
		},
	})
	a.mux.SetPaletteBase(configuredPaletteBase(a.cfg.Colors))
}

// fontInstallationPlan is an immutable description of the legacy startup font
// installation. It contains values only; resource ownership starts in prepare.
type fontInstallationPlan struct {
	spec        fontglyph.Spec
	textGamma   float64
	textDarken  float64
	fontFactory atlasBackendFactory
}

func newFontInstallationPlan(cfg config.Config, dpi float64, textRaster string, factory atlasBackendFactory) (fontInstallationPlan, error) {
	plan := fontInstallationPlan{
		spec: fontglyph.Spec{
			Family:     cfg.Font.Family,
			Size:       cfg.Font.Size,
			DPI:        dpi,
			TextRaster: textRaster,
		},
		textGamma:   cfg.Render.TextGamma,
		textDarken:  cfg.Render.TextDarken,
		fontFactory: factory,
	}
	if strings.TrimSpace(plan.spec.Family) == "" {
		return fontInstallationPlan{}, errors.New("font installation plan has empty family")
	}
	if plan.spec.Size <= 0 || plan.spec.DPI <= 0 {
		return fontInstallationPlan{}, fmt.Errorf("font installation plan has invalid size/DPI %.2f/%.2f", plan.spec.Size, plan.spec.DPI)
	}
	if factory == nil {
		return fontInstallationPlan{}, errors.New("nil atlas backend factory")
	}
	if _, err := makeAtlasFontKey(plan.spec, plan.textGamma, plan.textDarken); err != nil {
		return fontInstallationPlan{}, fmt.Errorf("font installation identity: %w", err)
	}
	return plan, nil
}

type fontInstallationMetrics struct {
	cellW, cellH int
	baseline     int
}

// fontInstallationStageSeam makes every fallible startup boundary directly
// testable without publishing resources into App state.
type fontInstallationStageSeam struct {
	plan    func(fontInstallationPlan) error
	reserve func(fontInstallationPlan) error
	load    func(fontInstallationPlan) (fontglyph.Backend, error)
	metrics func(fontglyph.Backend) (fontInstallationMetrics, error)
	context func(fontInstallationPlan, fontglyph.Backend, fontInstallationMetrics) (*atlasFontContext, error)
	adopt   func(*preparedFontInstallation) error
}

func defaultFontInstallationStages() fontInstallationStageSeam {
	return fontInstallationStageSeam{
		plan:    func(fontInstallationPlan) error { return nil },
		reserve: func(fontInstallationPlan) error { return nil },
		load: func(plan fontInstallationPlan) (fontglyph.Backend, error) {
			return plan.fontFactory(plan.spec)
		},
		metrics: func(backend fontglyph.Backend) (fontInstallationMetrics, error) {
			if backend == nil {
				return fontInstallationMetrics{}, errors.New("atlas backend factory returned nil backend")
			}
			cellW, cellH, baseline := backend.CellMetrics()
			if cellW <= 0 || cellH <= 0 || baseline < 0 || baseline > cellH {
				return fontInstallationMetrics{}, fmt.Errorf("invalid font cell metrics %dx%d baseline %d", cellW, cellH, baseline)
			}
			return fontInstallationMetrics{cellW: cellW, cellH: cellH, baseline: baseline}, nil
		},
		context: func(plan fontInstallationPlan, backend fontglyph.Backend, metrics fontInstallationMetrics) (*atlasFontContext, error) {
			return makeAtlasFontContextFromBackend(plan.spec, plan.textGamma, plan.textDarken, backend, metrics)
		},
		adopt: func(*preparedFontInstallation) error { return nil },
	}
}

// preparedFontInstallation owns its backend until successful adoption. Close is
// idempotent and never releases a backend after ownership transfers to the atlas.
type preparedFontInstallation struct {
	plan    fontInstallationPlan
	context *atlasFontContext
	backend fontglyph.Backend
	adopted bool
	closed  bool
}

func prepareFontInstallation(plan fontInstallationPlan, stages fontInstallationStageSeam) (_ *preparedFontInstallation, resultErr error) {
	if stages.plan == nil || stages.reserve == nil || stages.load == nil || stages.metrics == nil || stages.context == nil || stages.adopt == nil {
		return nil, errors.New("font installation stage seam is incomplete")
	}
	if err := stages.plan(plan); err != nil {
		return nil, fmt.Errorf("font installation plan: %w", err)
	}
	if err := stages.reserve(plan); err != nil {
		return nil, fmt.Errorf("font installation reserve: %w", err)
	}
	backend, err := stages.load(plan)
	if err != nil {
		return nil, fmt.Errorf("font installation load: %w", err)
	}
	if backend == nil {
		return nil, errors.New("font installation load returned nil backend")
	}
	prepared := &preparedFontInstallation{plan: plan, backend: backend}
	defer func() {
		if resultErr != nil {
			prepared.Close()
		}
	}()
	metrics, err := stages.metrics(backend)
	if err != nil {
		return nil, fmt.Errorf("font installation metrics: %w", err)
	}
	ctx, err := stages.context(plan, backend, metrics)
	if err != nil {
		return nil, fmt.Errorf("font installation context: %w", err)
	}
	if ctx == nil {
		return nil, errors.New("font installation context returned nil context")
	}
	prepared.context = ctx
	return prepared, nil
}

func (p *preparedFontInstallation) Close() {
	if p == nil || p.closed || p.adopted {
		return
	}
	p.closed = true
	if p.backend != nil {
		p.backend.Close()
	}
	p.backend = nil
	p.context = nil
}

func (p *preparedFontInstallation) adopt(r gpu.Renderer, stages fontInstallationStageSeam) (*glyphAtlas, error) {
	if p == nil || p.closed || p.adopted || p.context == nil || p.backend == nil {
		return nil, errors.New("font installation is not adoptable")
	}
	if r == nil {
		return nil, errors.New("nil atlas renderer")
	}
	if stages.adopt == nil {
		return nil, errors.New("font installation adopt stage is nil")
	}
	if err := stages.adopt(p); err != nil {
		return nil, fmt.Errorf("font installation adopt: %w", err)
	}
	atlas := newGlyphAtlasWithPreparedContext(r, p.context, p.plan.fontFactory)
	p.adopted = true
	p.backend = nil
	p.context = nil
	return atlas, nil
}
