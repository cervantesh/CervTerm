package fontglyph

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font/gofont/gomono"
)

// These compile-time assignments pin the synchronous compatibility facade.
// Discovery has no cancellation parameter or returned error: inaccessible roots
// are represented by diagnostics and a usable (possibly empty) index.
var (
	_ func([]string) *FontIndex   = BuildFontIndex
	_ func(string) FontResolution = ResolveSystemFont
	_ func() *FontIndex           = loadSystemFontIndex
	_ func() []string             = systemFontDirs
	_ func(string, int) string    = fontCacheKey
	_ func(string) string         = canonicalFontCacheSource
)

func TestL402DiscoveryFailureContract(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	index := BuildFontIndex([]string{missing, missing})
	if index == nil {
		t.Fatal("BuildFontIndex returned nil for inaccessible roots")
	}
	if got := index.Diagnostics(); got.Roots != 0 || got.FilesSkipped != 2 || got.CandidateFiles != 0 || got.FacesIndexed != 0 {
		t.Fatalf("inaccessible-root diagnostics = %+v", got)
	}
	if regular, bold, italic, boldItalic := index.Lookup("missing"); regular != nil || bold != nil || italic != nil || boldItalic != nil {
		t.Fatalf("missing lookup = %#v %#v %#v %#v", regular, bold, italic, boldItalic)
	}
}

func TestL402DiscoveryIdentityAndPlatformRootsContract(t *testing.T) {
	root := t.TempDir()
	fontPath := filepath.Join(root, "GoMono.ttf")
	writeTestFile(t, fontPath, gomono.TTF)

	index := BuildFontIndex([]string{root})
	regular, _, _, _ := index.Lookup("  GO   MONO ")
	if regular == nil {
		t.Fatal("normalized family lookup did not find Go Mono")
	}
	canonical, err := filepath.EvalSymlinks(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		t.Fatal(err)
	}
	canonical = filepath.Clean(canonical)
	if regular.path != canonical || regular.index != 0 || regular.metadata.CollectionIndex != 0 {
		t.Fatalf("discovered identity = path %q index %d metadata %+v; want %q#0", regular.path, regular.index, regular.metadata, canonical)
	}
	if got, want := fontCacheKey(regular.path, regular.index), canonicalFontCacheSource(canonical)+"#0"; got != want {
		t.Fatalf("cache identity = %q, want %q", got, want)
	}

	dirs := systemFontDirs()
	if len(dirs) == 0 {
		t.Fatal("systemFontDirs returned no platform roots")
	}
	if runtime.GOOS == "windows" {
		if base := filepath.Base(dirs[0]); base != "Fonts" {
			t.Fatalf("first Windows discovery root = %q, want Fonts directory", dirs[0])
		}
	} else if dirs[0] != "/usr/share/fonts" {
		t.Fatalf("first non-Windows discovery root = %q, want /usr/share/fonts", dirs[0])
	}
}

func TestL402CachedFaceIdentityAndOutputParity(t *testing.T) {
	manager := useTestParsedFaceCache(t, fontdesc.MaxParsedFaces, fontdesc.MaxParsedBytes)
	var loads atomic.Int32
	load := func() ([]byte, error) {
		loads.Add(1)
		return gomono.TTF, nil
	}
	first, _, err := loadCachedFaceIndexKnownSize("test:l402-output", 0, int64(len(gomono.TTF)), Spec{Family: "Go Mono", Size: 14, DPI: 96}, load)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := loadCachedFaceIndexKnownSize("test:l402-output", 0, int64(len(gomono.TTF)), Spec{Family: "Go Mono", Size: 14, DPI: 96}, load)
	if err != nil {
		releaseLoadedFace(first)
		t.Fatal(err)
	}
	defer releaseLoadedFace(first)
	defer releaseLoadedFace(second)
	if loads.Load() != 1 || first.sfnt != second.sfnt {
		t.Fatalf("loads=%d shared sfnt=%v", loads.Load(), first.sfnt == second.sfnt)
	}
	firstBounds, firstAdvance, firstOK := first.face.GlyphBounds('W')
	secondBounds, secondAdvance, secondOK := second.face.GlyphBounds('W')
	if firstOK != secondOK || firstBounds != secondBounds || firstAdvance != secondAdvance {
		t.Fatalf("cached face output drift: first=(%v,%v,%v) second=(%v,%v,%v)", firstBounds, firstAdvance, firstOK, secondBounds, secondAdvance, secondOK)
	}
	if got := manager.Stats().Pinned; got != 2 {
		t.Fatalf("cached face pins = %d, want 2", got)
	}
}

func BenchmarkL402BuildIndexGoMono(b *testing.B) {
	root := b.TempDir()
	writeTestFile(b, filepath.Join(root, "GoMono.ttf"), gomono.TTF)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		index := BuildFontIndex([]string{root})
		if got := index.Diagnostics(); got.FacesIndexed != 1 {
			b.Fatalf("diagnostics = %+v", got)
		}
	}
}

func BenchmarkL402CacheHitLease(b *testing.B) {
	manager := newParsedFaceCache(1, int64(len(gomono.TTF)))
	load := func() ([]byte, error) { return gomono.TTF, nil }
	_, seed, err := manager.Acquire("test:benchmark-hit", 0, int64(len(gomono.TTF)), load)
	if err != nil {
		b.Fatal(err)
	}
	seed.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, lease, err := manager.Acquire("test:benchmark-hit", 0, int64(len(gomono.TTF)), load)
		if err != nil {
			b.Fatal(err)
		}
		lease.Close()
	}
}

func writeTestFile(tb testing.TB, path string, data []byte) {
	tb.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		tb.Fatal(err)
	}
}
