package fontglyph

import (
	"image"
	"reflect"
	"sync/atomic"
	"testing"

	"cervterm/internal/fontglyph/cache"
	faceowner "cervterm/internal/fontglyph/internal/face"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/sfnt"
)

func useTestParsedFaceCache(t *testing.T, maxFaces int, maxBytes int64) *parsedFaceCache {
	t.Helper()
	manager := newParsedFaceCache(maxFaces, maxBytes)
	restore := resetParsedFaceCacheForTest(manager)
	t.Cleanup(restore)
	return manager
}

func releaseLoadedFace(face loadedFace) {
	face.cacheHandle.Close()
	if closer, ok := face.face.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

func TestFontParseCacheReusesParseAcrossSizes(t *testing.T) {
	manager := useTestParsedFaceCache(t, 4, int64(len(gomono.TTF))*2)
	var calls atomic.Int32
	load := func() ([]byte, error) {
		calls.Add(1)
		return gomono.TTF, nil
	}
	const key = "test:cache-reuse-fixture"
	f1, _, err := loadCachedFaceIndex(key, 0, Spec{Family: "Go Mono", Size: 12, DPI: 96}, load)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	defer releaseLoadedFace(f1)
	f2, _, err := loadCachedFaceIndex(key, 0, Spec{Family: "Go Mono", Size: 24, DPI: 96}, load)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	defer releaseLoadedFace(f2)
	if calls.Load() != 1 {
		t.Fatalf("loads = %d, want 1", calls.Load())
	}
	if f1.sfnt == nil || f1.sfnt != f2.sfnt {
		t.Fatal("faces do not share parsed sfnt")
	}
	if f1.face == f2.face {
		t.Fatal("point sizes unexpectedly share font.Face")
	}
	if got := manager.Stats().Pinned; got != 2 {
		t.Fatalf("pins = %d, want 2", got)
	}
}

func TestOpenTypeBackendCloseReleasesPinAndRejectsRaster(t *testing.T) {
	manager := useTestParsedFaceCache(t, 2, int64(len(gomono.TTF))*2)
	face, metrics, err := loadCachedFaceIndexKnownSize("test:backend-close", 0, int64(len(gomono.TTF)), Spec{Family: "Go Mono", Size: 12, DPI: 96}, func() ([]byte, error) { return gomono.TTF, nil })
	if err != nil {
		t.Fatal(err)
	}
	backend := &OpenTypeBackend{faces: []loadedFace{face}, cellW: 8, cellH: 16, baseline: metrics.Ascent.Ceil()}
	backend.Close()
	backend.Close()
	if got := manager.Stats(); got.Entries != 1 || got.Loading != 0 || got.Ready != 1 || got.Pinned != 0 || got.Bytes != int64(len(gomono.TTF)) {
		t.Fatalf("cache accounting after backend close = %+v, want one retained ready source with no pins and %d bytes", got, len(gomono.TTF))
	}
	if _, ok := backend.Rasterize('A', 1); ok {
		t.Fatal("closed backend rasterized a glyph")
	}
}

type closeOrderingFontFace struct {
	font.Face
	close func()
}

func (face *closeOrderingFontFace) Close() error {
	face.close()
	return nil
}

type closeOrderingGlyphRasterizer struct {
	close func()
}

func (*closeOrderingGlyphRasterizer) RasterizeGlyph(uint16, int, int, int, int, float32) (*image.RGBA, bool) {
	return nil, false
}

func (rasterizer *closeOrderingGlyphRasterizer) Close() { rasterizer.close() }

func TestOpenTypeBackendCloseReversesAcquisitionAndClearsRetainedFaces(t *testing.T) {
	manager := cache.New[*parsedFontData](2, 2, func([]byte, int) (*faceowner.Owner[*parsedFontData], error) {
		return faceowner.NewOwner(&parsedFontData{}, nil), nil
	})
	acquireLease := func(source string) *parsedFaceLease {
		t.Helper()
		_, lease, err := manager.Acquire(source, 0, 1, func() ([]byte, error) { return []byte{1}, nil })
		if err != nil {
			t.Fatalf("acquire %s: %v", source, err)
		}
		return lease
	}
	firstLease := acquireLease("test:close-order-first")
	secondLease := acquireLease("test:close-order-second")

	var events []string
	appendEvent := func(event string, wantPins int) {
		t.Helper()
		if got := manager.Stats().Pinned; got != wantPins {
			t.Fatalf("%s observed %d pins, want %d", event, got, wantPins)
		}
		events = append(events, event)
	}
	faces := []loadedFace{
		{
			face: &closeOrderingFontFace{close: func() { appendEvent("first-face", 1) }},
			sfnt: &sfnt.Font{}, tables: ColorTables{HasSVG: true},
			sbix: &sbixExtractor{data: []byte{1}}, cbdt: &cbdtExtractor{cbdt: []byte{1}},
			colr: &colrParser{data: []byte{1}}, svg: &svgExtractor{documents: []svgDocumentRecord{{document: []byte{1}}}},
			sourcePath: "first.ttf", faceIndex: 1, cacheHandle: firstLease,
		},
		{
			face: &closeOrderingFontFace{close: func() { appendEvent("second-face", 2) }},
			sfnt: &sfnt.Font{}, tables: ColorTables{HasSVG: true},
			sbix: &sbixExtractor{data: []byte{2}}, cbdt: &cbdtExtractor{cbdt: []byte{2}},
			colr: &colrParser{data: []byte{2}}, svg: &svgExtractor{documents: []svgDocumentRecord{{document: []byte{2}}}},
			sourcePath: "second.ttf", faceIndex: 2, cacheHandle: secondLease,
		},
	}
	retained := faces
	backend := &OpenTypeBackend{
		faces:    faces,
		dwRaster: &closeOrderingGlyphRasterizer{close: func() { appendEvent("dw-raster", 2) }},
	}
	backend.Close()

	if want := []string{"dw-raster", "second-face", "first-face"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("close order = %v, want %v", events, want)
	}
	if backend.dwRaster != nil || backend.faces != nil {
		t.Fatalf("backend retained resources after close: dw=%#v faces=%#v", backend.dwRaster, backend.faces)
	}
	for i, face := range retained {
		if face.face != nil || face.sfnt != nil || face.tables != (ColorTables{}) || face.sbix != nil || face.cbdt != nil || face.colr != nil || face.svg != nil || face.sourcePath != "" || face.faceIndex != 0 || face.cacheHandle != nil {
			t.Fatalf("retained face %d still references parsed/source-backed state: %#v", i, face)
		}
	}
	if got := manager.Stats(); got.Entries != 2 || got.Loading != 0 || got.Ready != 2 || got.Pinned != 0 || got.Bytes != 2 {
		t.Fatalf("cache accounting after close = %+v, want two retained ready sources with no pins and 2 bytes", got)
	}
	backend.Close()
	if want := []string{"dw-raster", "second-face", "first-face"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("idempotent close order = %v, want %v", events, want)
	}
}
