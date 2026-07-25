package fontglyph

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	_ func([]string) *FontIndex          = BuildFontIndex
	_ func(string) FontResolution        = ResolveSystemFont
	_ func() *FontIndex                  = loadSystemFontIndex
	_ func() []string                    = systemFontDirs
	_ func(string, int) string           = fontCacheKey
	_ func(string) string                = canonicalFontCacheSource
	_ func([]string, int) []string       = selectTopKPaths
	_ func(string) []faceInfo            = fontFaces
	_ func(string) (bool, bool)          = classifySubfamily
	_ func(string) string                = normalizeFamily
	_ func(string) bool                  = isFontFile
	_ func(string, []string) bool        = pathWithinRoots
	_ func(string, string) int           = compareDiscoveryPaths
	_ func(string) string                = discoveryPathKey
	_ func(int) *topKPathSelector        = newTopKPathSelector
	_ func(int, int64) *fontCacheManager = newFontCacheManager
)

func TestL402RootCompatibilityTypeAndMethodInventory(t *testing.T) {
	faceType := reflect.TypeOf(faceInfo{})
	if faceType.Name() != "faceInfo" || faceType.NumMethod() != 0 {
		t.Fatalf("face compatibility identity = %q with %d methods", faceType.Name(), faceType.NumMethod())
	}
	fields := make([]string, faceType.NumField())
	for i := range fields {
		fields[i] = faceType.Field(i).Name
	}
	if want := []string{"path", "index", "family", "subfamily", "metadata"}; !reflect.DeepEqual(fields, want) {
		t.Fatalf("faceInfo fields = %v, want %v", fields, want)
	}

	indexType := reflect.TypeOf(FontIndex{})
	if indexType.Name() != "FontIndex" {
		t.Fatalf("index compatibility identity = %q, want FontIndex", indexType.Name())
	}
	pointerType := reflect.PointerTo(indexType)
	methods := make([]string, pointerType.NumMethod())
	for i := range methods {
		methods[i] = pointerType.Method(i).Name
	}
	if want := []string{"Diagnostics", "Lookup"}; !reflect.DeepEqual(methods, want) {
		t.Fatalf("FontIndex method set = %v, want %v", methods, want)
	}
}

func TestL402DiscoveryFailureAndOrderingContract(t *testing.T) {
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

	paths := []string{
		filepath.Join("z", "font.ttf"),
		filepath.Join("A", "font.ttf"),
		filepath.Join("m", "font.ttf"),
		filepath.Join("b", "font.ttf"),
	}
	forward := selectTopKPaths(paths, 3)
	reversed := selectTopKPaths([]string{paths[3], paths[2], paths[1], paths[0]}, 3)
	if !reflect.DeepEqual(forward, reversed) {
		t.Fatalf("top-K depends on traversal order: forward=%v reversed=%v", forward, reversed)
	}
	for i := 1; i < len(forward); i++ {
		if compareDiscoveryPaths(forward[i-1], forward[i]) >= 0 {
			t.Fatalf("top-K output is not strict canonical order: %v", forward)
		}
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
	if discoveryPathKey(regular.path) != discoveryPathKey(canonical) || regular.index != 0 || regular.metadata.CollectionIndex != 0 {
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

func TestL402CacheOnePinPerHandleAndIdempotentRelease(t *testing.T) {
	manager := newFontCacheManager(2, 16)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	load := func() ([]byte, error) { return []byte{1, 2, 3, 4}, nil }

	_, first, err := manager.acquire("test:l402-pin", 0, 4, load)
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.stats(); got.Pinned != 1 || got.Entries != 1 || got.Bytes != 4 {
		t.Fatalf("first lease stats = %+v", got)
	}
	_, second, err := manager.acquire("test:l402-pin", 0, 4, func() ([]byte, error) {
		return nil, errors.New("cache hit unexpectedly loaded source")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.stats().Pinned; got != 2 {
		t.Fatalf("two handles pin count = %d, want 2", got)
	}
	first.release()
	first.release()
	if got := manager.stats().Pinned; got != 1 {
		t.Fatalf("idempotent first release pin count = %d, want 1", got)
	}
	second.release()
	second.release()
	if got := manager.stats().Pinned; got != 0 {
		t.Fatalf("idempotent second release pin count = %d, want 0", got)
	}
}

func TestL402CacheDeterministicTieEviction(t *testing.T) {
	manager := newFontCacheManager(2, 16)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	load := func(value byte) func() ([]byte, error) {
		return func() ([]byte, error) { return []byte{value}, nil }
	}
	_, z, err := manager.acquire("test:z", 0, 1, load('z'))
	if err != nil {
		t.Fatal(err)
	}
	_, a, err := manager.acquire("test:a", 0, 1, load('a'))
	if err != nil {
		t.Fatal(err)
	}
	z.release()
	a.release()

	manager.mu.Lock()
	manager.entries[fontCacheKey("test:z", 0)].lastUsed = 7
	manager.entries[fontCacheKey("test:a", 0)].lastUsed = 7
	manager.mu.Unlock()

	_, next, err := manager.acquire("test:next", 0, 1, load('n'))
	if err != nil {
		t.Fatal(err)
	}
	next.release()
	manager.mu.Lock()
	_, hasA := manager.entries[fontCacheKey("test:a", 0)]
	_, hasZ := manager.entries[fontCacheKey("test:z", 0)]
	manager.mu.Unlock()
	if hasA || !hasZ {
		t.Fatalf("tie eviction hasA=%v hasZ=%v, want lexical a evicted", hasA, hasZ)
	}
}

func TestL402CachedFaceIdentityAndOutputParity(t *testing.T) {
	manager := useTestFontCache(t, fontdesc.MaxParsedFaces, fontdesc.MaxParsedBytes)
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
	if got := manager.stats().Pinned; got != 2 {
		t.Fatalf("cached face pins = %d, want 2", got)
	}
}

func BenchmarkL402DiscoveryTopK(b *testing.B) {
	paths := make([]string, fontdesc.MaxDiscoveryFiles+257)
	for i := range paths {
		paths[i] = filepath.Join("fonts", benchmarkPathName(len(paths)-i))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if got := selectTopKPaths(paths, fontdesc.MaxDiscoveryFiles); len(got) != fontdesc.MaxDiscoveryFiles {
			b.Fatalf("selected %d paths", len(got))
		}
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
	manager := newFontCacheManager(1, 8)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	load := func() ([]byte, error) { return []byte{1}, nil }
	_, seed, err := manager.acquire("test:benchmark-hit", 0, 1, load)
	if err != nil {
		b.Fatal(err)
	}
	seed.release()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, handle, err := manager.acquire("test:benchmark-hit", 0, 1, load)
		if err != nil {
			b.Fatal(err)
		}
		handle.release()
	}
}

func benchmarkPathName(value int) string {
	const digits = "0123456789abcdef"
	var name [12]byte
	for i := len(name) - 5; i >= 0; i-- {
		name[i] = digits[value&15]
		value >>= 4
	}
	copy(name[len(name)-4:], ".ttf")
	return string(name[:])
}

func writeTestFile(tb testing.TB, path string, data []byte) {
	tb.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		tb.Fatal(err)
	}
}
