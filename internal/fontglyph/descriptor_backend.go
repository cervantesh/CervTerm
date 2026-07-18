package fontglyph

import (
	"errors"
	"fmt"
	"sync"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font"
)

// styledFontBackend extends Backend with explicit requested-style resolution
// and rasterization. Callers that only know Backend retain normal-style behavior.
type styledFontBackend interface {
	Backend
	SupportsLigatures() bool
	StyleResolution(request fontdesc.RequestedFaceStyle) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool)
	RasterizeStyle(request fontdesc.RequestedFaceStyle, r rune, cellSpan int) (RasterizedGlyph, bool)
	RasterizeClusterStyle(request fontdesc.RequestedFaceStyle, cluster string, cellSpan int) (RasterizedGlyph, bool)
	RasterizeRun(run string, cellSpan int) (RasterizedGlyph, bool)
	RasterizeRunStyle(request fontdesc.RequestedFaceStyle, run string, cellSpan int) (RasterizedGlyph, bool)
}

// FaceDiagnostic is a path-free snapshot of one concrete selected face.
// It is intended for one-shot diagnostic probes and never exposes cache keys or files.
type FaceDiagnostic struct {
	Metadata      fontdesc.FaceMetadata
	Tier          fontdesc.SourceTier
	AuthoredIndex uint32
	Synthetic     fontdesc.SyntheticMode
}

func diagnosticFace(plan resolvedFacePlan) FaceDiagnostic {
	return FaceDiagnostic{
		Metadata:      plan.selected.metadata.Normalized(),
		Tier:          plan.tier,
		AuthoredIndex: plan.authoredIndex,
		Synthetic:     plan.synthetic,
	}
}

// DiagnosticStyleFace reports the selected primary face for one requested style.
// The returned value is detached and path-free.
func DiagnosticStyleFace(backend Backend, request fontdesc.RequestedFaceStyle) (FaceDiagnostic, bool) {
	switch typed := backend.(type) {
	case *descriptorBackend:
		if _, ok := typed.backendForStyle(request); !ok {
			return FaceDiagnostic{}, false
		}
		return diagnosticFace(typed.plans[request]), true
	case *fallbackBackend:
		if typed == nil || typed.closed || typed.primary == nil {
			return FaceDiagnostic{}, false
		}
		return DiagnosticStyleFace(typed.primary, request)
	default:
		return FaceDiagnostic{}, false
	}
}

// DiagnosticContentFace resolves one representative cluster through the normal
// lazy rule/primary/fallback/embedded order and returns only path-free metadata.
func DiagnosticContentFace(backend Backend, request fontdesc.RequestedFaceStyle, content string) (FaceDiagnostic, bool) {
	if content == "" {
		return FaceDiagnostic{}, false
	}
	if fallback, ok := backend.(*fallbackBackend); ok {
		selection, resolved := fallback.resolveContent(request, content)
		if !resolved {
			return FaceDiagnostic{}, false
		}
		return diagnosticFace(selection.plan), true
	}
	return FaceDiagnostic{}, false
}

type descriptorBackend struct {
	backends  [4]*OpenTypeBackend
	plans     [4]resolvedFacePlan
	closed    bool
	closeOnce sync.Once
}

type resolvedFacePlanLoader func(Spec, resolvedFacePlan) (loadedFace, font.Metrics, error)

// NewDescriptorBackend prepares all requested primary styles from the process-wide
// system font index. The supplied environment must describe the same ordered
// descriptor set and raster spec used by the caller's atlas identity.
func NewDescriptorBackend(spec Spec, environment fontdesc.FontEnvironmentKey, descriptors []fontdesc.Descriptor) (Backend, error) {
	if err := validateDescriptorBackendDescriptors(descriptors); err != nil {
		return nil, err
	}
	return newDescriptorBackend(spec, environment, descriptors, loadSystemFontIndex())
}

func newDescriptorBackend(spec Spec, environment fontdesc.FontEnvironmentKey, descriptors []fontdesc.Descriptor, index *FontIndex) (*descriptorBackend, error) {
	return newDescriptorBackendWithLoader(spec, environment, descriptors, index, loadResolvedFacePlan)
}

func validateDescriptorBackendDescriptors(descriptors []fontdesc.Descriptor) error {
	if len(descriptors) == 0 {
		return errors.New("descriptor backend requires at least one primary descriptor")
	}
	if len(descriptors) > fontdesc.MaxPrimaryDescriptors {
		return fmt.Errorf("primary descriptor count %d exceeds %d", len(descriptors), fontdesc.MaxPrimaryDescriptors)
	}
	return nil
}

func newDescriptorBackendWithLoader(spec Spec, environment fontdesc.FontEnvironmentKey, descriptors []fontdesc.Descriptor, index *FontIndex, load resolvedFacePlanLoader) (*descriptorBackend, error) {
	if err := validateDescriptorBackendDescriptors(descriptors); err != nil {
		return nil, err
	}
	backend := &descriptorBackend{}
	for request := fontdesc.RequestedFaceStyleNormal; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
		plans, resolutionErr := resolvePrimaryFacePlan(index, environment, descriptors, request)
		embedded, embeddedErr := resolveEmbeddedFallbackPlan(environment, request)
		if embeddedErr == nil {
			plans = append(plans, embedded)
		}

		attemptErrors := make([]error, 0, len(plans)+2)
		if resolutionErr != nil {
			attemptErrors = append(attemptErrors, resolutionErr)
		}
		if embeddedErr != nil {
			attemptErrors = append(attemptErrors, embeddedErr)
		}

		var selected *OpenTypeBackend
		var selectedPlan resolvedFacePlan
		for _, plan := range plans {
			primary, metrics, err := load(spec, plan)
			if err != nil {
				attemptErrors = append(attemptErrors, fmt.Errorf("source %q face %d: %w", plan.selected.path, plan.selected.index, err))
				continue
			}
			selected = newOpenTypeBackendFromPrimary(spec, primary, metrics)
			selectedPlan = plan
			break
		}
		if selected == nil {
			backend.Close()
			return nil, fmt.Errorf("prepare requested style %d: %w", request, errors.Join(attemptErrors...))
		}
		backend.backends[request] = selected
		backend.plans[request] = selectedPlan
	}
	normal := backend.backends[fontdesc.RequestedFaceStyleNormal]
	for request := fontdesc.RequestedFaceStyleBold; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
		backend.backends[request].cellW = normal.cellW
		backend.backends[request].cellH = normal.cellH
		backend.backends[request].baseline = normal.baseline
	}
	return backend, nil
}

func loadResolvedFacePlan(spec Spec, plan resolvedFacePlan) (loadedFace, font.Metrics, error) {
	var (
		face    loadedFace
		metrics font.Metrics
		err     error
	)
	if plan.selected.path == "embedded:gomono" && plan.selected.index == 0 {
		face, metrics, err = embeddedGoMono(spec)
	} else {
		face, metrics, err = loadCachedFileFaceIndex(plan.selected.path, plan.selected.index, spec)
	}
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	face.sourcePath = plan.selected.path
	face.faceIndex = plan.selected.index
	return face, metrics, nil
}

func (b *descriptorBackend) CellMetrics() (width int, height int, baseline int) {
	backend, ok := b.backendForStyle(fontdesc.RequestedFaceStyleNormal)
	if !ok {
		return 0, 0, 0
	}
	return backend.CellMetrics()
}

func (b *descriptorBackend) TextRasterEngine() string {
	backend, ok := b.backendForStyle(fontdesc.RequestedFaceStyleNormal)
	if !ok {
		return "go"
	}
	return backend.TextRasterEngine()
}

func (b *descriptorBackend) Rasterize(r rune, cellSpan int) (RasterizedGlyph, bool) {
	return b.RasterizeStyle(fontdesc.RequestedFaceStyleNormal, r, cellSpan)
}

func (b *descriptorBackend) RasterizeCluster(cluster string, cellSpan int) (RasterizedGlyph, bool) {
	return b.RasterizeClusterStyle(fontdesc.RequestedFaceStyleNormal, cluster, cellSpan)
}

func (b *descriptorBackend) SupportsLigatures() bool {
	backend, ok := b.backendForStyle(fontdesc.RequestedFaceStyleNormal)
	return ok && backend.SupportsLigatures()
}

func (b *descriptorBackend) RasterizeRun(run string, cellSpan int) (RasterizedGlyph, bool) {
	return b.RasterizeRunStyle(fontdesc.RequestedFaceStyleNormal, run, cellSpan)
}

func (b *descriptorBackend) StyleResolution(request fontdesc.RequestedFaceStyle) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool) {
	if _, ok := b.backendForStyle(request); !ok {
		return fontdesc.ResolvedFaceKey{}, fontdesc.SyntheticNone, false
	}
	plan := b.plans[request]
	return plan.resolvedKey, plan.synthetic, true
}

func (b *descriptorBackend) RasterizeStyle(request fontdesc.RequestedFaceStyle, r rune, cellSpan int) (RasterizedGlyph, bool) {
	backend, ok := b.backendForStyle(request)
	if !ok {
		return RasterizedGlyph{}, false
	}
	return backend.Rasterize(r, cellSpan)
}

func (b *descriptorBackend) RasterizeClusterStyle(request fontdesc.RequestedFaceStyle, cluster string, cellSpan int) (RasterizedGlyph, bool) {
	backend, ok := b.backendForStyle(request)
	if !ok {
		return RasterizedGlyph{}, false
	}
	return backend.RasterizeCluster(cluster, cellSpan)
}

func (b *descriptorBackend) RasterizeRunStyle(request fontdesc.RequestedFaceStyle, run string, cellSpan int) (RasterizedGlyph, bool) {
	backend, ok := b.backendForStyle(request)
	if !ok {
		return RasterizedGlyph{}, false
	}
	return backend.RasterizeRun(run, cellSpan)
}

func (b *descriptorBackend) backendForStyle(request fontdesc.RequestedFaceStyle) (*OpenTypeBackend, bool) {
	if b == nil || b.closed || request > fontdesc.RequestedFaceStyleBoldItalic {
		return nil, false
	}
	backend := b.backends[request]
	return backend, backend != nil
}

func (b *descriptorBackend) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		b.closed = true
		for i := range b.backends {
			if b.backends[i] != nil {
				b.backends[i].Close()
				b.backends[i] = nil
			}
		}
	})
}

var _ styledFontBackend = (*descriptorBackend)(nil)
