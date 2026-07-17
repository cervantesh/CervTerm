package fontglyph

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/math/fixed"
)

func TestDescriptorBackendResolvesRealAndSyntheticRequestedStyles(t *testing.T) {
	t.Run("real", func(t *testing.T) {
		dir := t.TempDir()
		faces := []faceInfo{
			descriptorTestFace(t, dir, "regular.ttf", "Test Mono", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
			descriptorTestFace(t, dir, "bold.ttf", "Test Mono", "Bold", 700, fontdesc.StyleNormal, 0, gomono.TTF),
			descriptorTestFace(t, dir, "italic.ttf", "Test Mono", "Italic", 400, fontdesc.StyleItalic, 0, gomono.TTF),
			descriptorTestFace(t, dir, "bold-italic.ttf", "Test Mono", "Bold Italic", 700, fontdesc.StyleItalic, 0, gomono.TTF),
		}
		descriptors := []fontdesc.Descriptor{{Family: "Test Mono"}}
		backend := newDescriptorTestBackend(t, descriptors, faces)
		defer backend.Close()

		wantSources := []string{faces[0].path, faces[1].path, faces[2].path, faces[3].path}
		keys := make(map[fontdesc.ResolvedFaceKey]struct{}, 4)
		for request := fontdesc.RequestedFaceStyleNormal; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
			key, synthetic, ok := backend.StyleResolution(request)
			if !ok || synthetic != fontdesc.SyntheticNone {
				t.Fatalf("StyleResolution(%d) = %v, %d, %v; want real face", request, key, synthetic, ok)
			}
			if got := backend.plans[request].selected.path; got != canonicalFontCacheSource(wantSources[request]) {
				t.Fatalf("style %d source = %q, want %q", request, got, canonicalFontCacheSource(wantSources[request]))
			}
			keys[key] = struct{}{}
		}
		if len(keys) != 4 {
			t.Fatalf("resolved style keys are not distinct: %v", keys)
		}
	})

	t.Run("synthetic", func(t *testing.T) {
		dir := t.TempDir()
		face := descriptorTestFace(t, dir, "regular.ttf", "Synthetic Mono", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF)
		descriptors := []fontdesc.Descriptor{{Family: "Synthetic Mono"}}
		backend := newDescriptorTestBackend(t, descriptors, []faceInfo{face})
		defer backend.Close()

		want := []fontdesc.SyntheticMode{
			fontdesc.SyntheticNone,
			fontdesc.SyntheticBold,
			fontdesc.SyntheticItalic,
			fontdesc.SyntheticBold | fontdesc.SyntheticItalic,
		}
		for request := fontdesc.RequestedFaceStyleNormal; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
			_, synthetic, ok := backend.StyleResolution(request)
			if !ok || synthetic != want[request] {
				t.Fatalf("StyleResolution(%d) synthetic = %d, %v; want %d, true", request, synthetic, ok, want[request])
			}
		}
	})
}

func TestDescriptorBackendContinuesAfterUnusableCandidates(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "missing"},
		{name: "corrupt", data: []byte("not a font")},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			badPath := filepath.Join(dir, test.name+".ttf")
			if test.data != nil {
				if err := os.WriteFile(badPath, test.data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			good := descriptorTestFace(t, dir, "good.ttf", "Good Mono", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF)
			bad := faceInfo{path: badPath, index: 0, family: "Bad Mono", subfamily: "Regular", metadata: fontdesc.FaceMetadata{Family: "Bad Mono", Subfamily: "Regular", Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100}}
			descriptors := []fontdesc.Descriptor{{Family: "Bad Mono"}, {Family: "Good Mono"}}
			backend := newDescriptorTestBackend(t, descriptors, []faceInfo{bad, good})
			defer backend.Close()
			for request := fontdesc.RequestedFaceStyleNormal; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
				if got := backend.plans[request].selected.path; got != canonicalFontCacheSource(good.path) {
					t.Fatalf("style %d selected %q after bad candidate, want %q", request, got, canonicalFontCacheSource(good.path))
				}
			}
		})
	}
}

func TestDescriptorBackendLoadsNonzeroCollectionFace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "two-face.ttc")
	if err := os.WriteFile(path, makeTestTTC(t, gomono.TTF, gomono.TTF), 0o600); err != nil {
		t.Fatal(err)
	}
	face := faceInfo{
		path: path, index: 1, family: "Collection Mono", subfamily: "Second",
		metadata: fontdesc.FaceMetadata{Family: "Collection Mono", Subfamily: "Second", Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100, CollectionIndex: 1},
	}
	descriptors := []fontdesc.Descriptor{{Family: "Collection Mono", CollectionIndex: fontdesc.SomeCollectionIndex(1)}}
	backend := newDescriptorTestBackend(t, descriptors, []faceInfo{face})
	defer backend.Close()
	for request := fontdesc.RequestedFaceStyleNormal; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
		if backend.plans[request].selected.index != 1 || backend.backends[request].faces[0].faceIndex != 1 {
			t.Fatalf("style %d did not retain collection face 1", request)
		}
		if got := backend.backends[request].faces[0].sourcePath; got != canonicalFontCacheSource(path) {
			t.Fatalf("style %d sourcePath = %q, want %q", request, got, canonicalFontCacheSource(path))
		}
	}
}

func TestDescriptorBackendProjectsStyleMetricsToNormalGrid(t *testing.T) {
	dir := t.TempDir()
	face := descriptorTestFace(t, dir, "regular.ttf", "Grid Mono", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF)
	descriptors := []fontdesc.Descriptor{{Family: "Grid Mono"}}
	calls := 0
	loader := func(spec Spec, plan resolvedFacePlan) (loadedFace, font.Metrics, error) {
		primary, metrics, err := loadResolvedFacePlan(spec, plan)
		if err == nil {
			metrics.Height += fixed.I(calls)
			metrics.Ascent += fixed.I(calls)
			calls++
		}
		return primary, metrics, err
	}
	backend, err := newDescriptorBackendWithLoader(
		Spec{Family: "ignored", Size: 14, DPI: 96},
		descriptorTestEnvironment(t, descriptors),
		descriptors,
		descriptorTestIndex([]faceInfo{face}),
		loader,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	wantW, wantH, wantBaseline := backend.backends[fontdesc.RequestedFaceStyleNormal].CellMetrics()
	for request := fontdesc.RequestedFaceStyleBold; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
		if gotW, gotH, gotBaseline := backend.backends[request].CellMetrics(); gotW != wantW || gotH != wantH || gotBaseline != wantBaseline {
			t.Fatalf("style %d metrics = %d,%d,%d; want normal %d,%d,%d", request, gotW, gotH, gotBaseline, wantW, wantH, wantBaseline)
		}
	}
}

func TestDescriptorBackendRollbackAndCloseReleaseCachePins(t *testing.T) {
	manager := newFontCacheManager(fontdesc.MaxParsedFaces, fontdesc.MaxParsedBytes)
	restore := resetFontCacheForTest(manager)
	defer restore()

	dir := t.TempDir()
	face := descriptorTestFace(t, dir, "regular.ttf", "Rollback Mono", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF)
	descriptors := []fontdesc.Descriptor{{Family: "Rollback Mono"}}
	environment := descriptorTestEnvironment(t, descriptors)
	index := descriptorTestIndex([]faceInfo{face})
	loads := 0
	loader := func(spec Spec, plan resolvedFacePlan) (loadedFace, font.Metrics, error) {
		loads++
		if loads > 1 {
			return loadedFace{}, font.Metrics{}, errors.New("injected style load failure")
		}
		return loadResolvedFacePlan(spec, plan)
	}
	backend, err := newDescriptorBackendWithLoader(Spec{Family: "ignored", Size: 14, DPI: 96}, environment, descriptors, index, loader)
	if err == nil || backend != nil || !strings.Contains(err.Error(), "prepare requested style 1") || !strings.Contains(err.Error(), "injected style load failure") {
		t.Fatalf("rollback constructor = %#v, %v", backend, err)
	}
	if stats := manager.stats(); stats.Pinned != 0 {
		t.Fatalf("cache pins after constructor rollback = %d, want 0", stats.Pinned)
	}

	backend = newDescriptorTestBackendWithManager(t, descriptors, []faceInfo{face}, manager)
	if stats := manager.stats(); stats.Pinned != 4 {
		t.Fatalf("cache pins before close = %d, want 4", stats.Pinned)
	}
	backend.Close()
	backend.Close()
	if stats := manager.stats(); stats.Pinned != 0 {
		t.Fatalf("cache pins after idempotent close = %d, want 0", stats.Pinned)
	}
}

func TestDescriptorBackendDefaultMethodsDelegateNormalAndRejectInvalidStyle(t *testing.T) {
	dir := t.TempDir()
	face := descriptorTestFace(t, dir, "regular.ttf", "Delegate Mono", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF)
	backend := newDescriptorTestBackend(t, []fontdesc.Descriptor{{Family: "Delegate Mono"}}, []faceInfo{face})
	defer backend.Close()

	defaultGlyph, defaultOK := backend.Rasterize('A', 1)
	styleGlyph, styleOK := backend.RasterizeStyle(fontdesc.RequestedFaceStyleNormal, 'A', 1)
	if !defaultOK || !styleOK || defaultGlyph.Image == nil || styleGlyph.Image == nil || !bytes.Equal(defaultGlyph.Image.Pix, styleGlyph.Image.Pix) {
		t.Fatal("default Rasterize did not delegate to normal style")
	}
	defaultCluster, defaultClusterOK := backend.RasterizeCluster("A", 1)
	styleCluster, styleClusterOK := backend.RasterizeClusterStyle(fontdesc.RequestedFaceStyleNormal, "A", 1)
	if !defaultClusterOK || !styleClusterOK || defaultCluster.Image == nil || styleCluster.Image == nil || !bytes.Equal(defaultCluster.Image.Pix, styleCluster.Image.Pix) {
		t.Fatal("default RasterizeCluster did not delegate to normal style")
	}

	invalid := fontdesc.RequestedFaceStyle(255)
	if _, _, ok := backend.StyleResolution(invalid); ok {
		t.Fatal("invalid style unexpectedly resolved")
	}
	if _, ok := backend.RasterizeStyle(invalid, 'A', 1); ok {
		t.Fatal("invalid style unexpectedly rasterized")
	}
	if _, ok := backend.RasterizeClusterStyle(invalid, "A", 1); ok {
		t.Fatal("invalid style unexpectedly rasterized a cluster")
	}
}

func TestNewDescriptorBackendRejectsEmptyAndTooManyDescriptorsBeforeSystemDiscovery(t *testing.T) {
	spec := Spec{Family: "ignored", Size: 14, DPI: 96}
	if backend, err := NewDescriptorBackend(spec, fontdesc.FontEnvironmentKey{}, nil); backend != nil || err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("empty descriptors = %#v, %v", backend, err)
	}
	tooMany := make([]fontdesc.Descriptor, fontdesc.MaxPrimaryDescriptors+1)
	for i := range tooMany {
		tooMany[i] = fontdesc.Descriptor{Family: "Test Mono"}
	}
	if backend, err := NewDescriptorBackend(spec, fontdesc.FontEnvironmentKey{}, tooMany); backend != nil || err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("too many descriptors = %#v, %v", backend, err)
	}
}

func newDescriptorTestBackend(t *testing.T, descriptors []fontdesc.Descriptor, faces []faceInfo) *descriptorBackend {
	t.Helper()
	manager := newFontCacheManager(fontdesc.MaxParsedFaces, fontdesc.MaxParsedBytes)
	restore := resetFontCacheForTest(manager)
	t.Cleanup(restore)
	return newDescriptorTestBackendWithManager(t, descriptors, faces, manager)
}

func newDescriptorTestBackendWithManager(t *testing.T, descriptors []fontdesc.Descriptor, faces []faceInfo, _ *fontCacheManager) *descriptorBackend {
	t.Helper()
	backend, err := newDescriptorBackend(Spec{Family: "ignored", Size: 14, DPI: 96}, descriptorTestEnvironment(t, descriptors), descriptors, descriptorTestIndex(faces))
	if err != nil {
		t.Fatalf("newDescriptorBackend: %v", err)
	}
	return backend
}

func descriptorTestEnvironment(t *testing.T, descriptors []fontdesc.Descriptor) fontdesc.FontEnvironmentKey {
	t.Helper()
	key, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: descriptors, DPI: 96})
	if err != nil {
		t.Fatalf("NewFontEnvironmentKey: %v", err)
	}
	return key
}

func descriptorTestIndex(faces []faceInfo) *FontIndex {
	index := &FontIndex{families: make(map[string][]faceInfo)}
	for _, face := range faces {
		index.families[normalizeFamily(face.family)] = append(index.families[normalizeFamily(face.family)], face)
	}
	return index
}

func descriptorTestFace(t *testing.T, dir, filename, family, subfamily string, weight int, style fontdesc.Style, index int, data []byte) faceInfo {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return faceInfo{
		path: path, index: index, family: family, subfamily: subfamily,
		metadata: fontdesc.FaceMetadata{Family: family, Subfamily: subfamily, Weight: weight, Style: style, Stretch: 100, CollectionIndex: uint32(index)},
	}
}
