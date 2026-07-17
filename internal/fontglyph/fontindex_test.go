package fontglyph

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

func TestFontNameExtraction(t *testing.T) {
	faces := fontFaces(filepath.Join("testdata", "noto-color-emoji-smoke.ttf"))
	if len(faces) == 0 {
		t.Fatal("fixture produced no named faces")
	}
	if faces[0].family == "" || faces[0].subfamily == "" {
		t.Fatalf("fixture names = family %q, subfamily %q", faces[0].family, faces[0].subfamily)
	}
}

func TestNormalizeFamily(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Cascadia Mono", "cascadia mono"},
		{"  CASCADIA   mono  ", "cascadia mono"},
		{"Go Mono", "go mono"},
	}
	for _, tt := range tests {
		if got := normalizeFamily(tt.input); got != tt.want {
			t.Errorf("normalizeFamily(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFontIndexLookup(t *testing.T) {
	index := &FontIndex{families: map[string][]faceInfo{
		"example mono": {
			{path: "regular.ttc", index: 3, family: "Example Mono", subfamily: "Book"},
			{path: "bold.ttc", index: 4, family: "Example Mono", subfamily: "Bold"},
			{path: "italic.ttc", index: 5, family: "Example Mono", subfamily: "Oblique"},
			{path: "bold-italic.ttc", index: 6, family: "Example Mono", subfamily: "Italic Bold"},
		},
	}}
	regular, bold, italic, boldItalic := index.Lookup(" EXAMPLE   MONO ")
	if regular == nil || regular.path != "regular.ttc" || regular.index != 3 || bold == nil || bold.index != 4 || italic == nil || italic.index != 5 || boldItalic == nil || boldItalic.index != 6 {
		t.Fatalf("Lookup variants = %#v %#v %#v %#v", regular, bold, italic, boldItalic)
	}
}

func TestMissingFamilyFallsBack(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "CervTerm Definitely Missing Family", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend() error = %v", err)
	}
	if backend == nil {
		t.Fatal("NewOpenTypeBackend() returned nil")
	}
	backend.Close()
}

func TestSelectTopKPathsIndependentOfTraversalOrder(t *testing.T) {
	first := []string{"z.ttf", "c.ttf", "a.ttf", "m.ttf", "b.ttf"}
	second := []string{"b.ttf", "m.ttf", "a.ttf", "c.ttf", "z.ttf"}
	want := []string{"a.ttf", "b.ttf", "c.ttf"}
	if got := selectTopKPaths(first, 3); !reflect.DeepEqual(got, want) {
		t.Fatalf("first selection = %v, want %v", got, want)
	}
	if got := selectTopKPaths(second, 3); !reflect.DeepEqual(got, want) {
		t.Fatalf("second selection = %v, want %v", got, want)
	}
	if got := selectTopKPaths(first, 0); len(got) != 0 {
		t.Fatalf("zero selection = %v", got)
	}
}

func TestBuildFontIndexCanonicalizesAndDeduplicatesRoots(t *testing.T) {
	root := t.TempDir()
	fontPath := filepath.Join(root, "GoMono.ttf")
	if err := os.WriteFile(fontPath, gomono.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	index := BuildFontIndex([]string{root, filepath.Join(root, ".")})
	regular, _, _, _ := index.Lookup("Go Mono")
	if regular == nil {
		t.Fatal("Go Mono was not indexed")
	}
	canonical, err := filepath.EvalSymlinks(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	if regular.path != canonical {
		t.Fatalf("indexed path = %q, want canonical %q", regular.path, canonical)
	}
	diagnostics := index.Diagnostics()
	if diagnostics.Roots != 1 || diagnostics.CandidateFiles != 1 || diagnostics.SelectedFiles != 1 || diagnostics.FacesExamined != 1 || diagnostics.FacesIndexed != 1 {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestBuildFontIndexSymlinkPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Developer-mode/privilege availability varies; attempt below and skip on
		// the actual operation rather than assuming it is available.
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	insideFont := filepath.Join(root, "fonts", "inside.ttf")
	outsideFont := filepath.Join(outside, "outside.ttf")
	for _, path := range []string{insideFont, outsideFont} {
		if err := os.WriteFile(path, gomono.TTF, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(insideFont, filepath.Join(root, "inside-link.ttf")); err != nil {
		t.Skipf("file symlinks unavailable: %v", err)
	}
	if err := os.Symlink(outsideFont, filepath.Join(root, "outside-link.ttf")); err != nil {
		t.Skipf("file symlinks unavailable: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside-dir")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	index := BuildFontIndex([]string{root})
	diagnostics := index.Diagnostics()
	if diagnostics.CandidateFiles != 1 || diagnostics.DuplicateFiles != 1 {
		t.Fatalf("inside symlink was not canonicalized/deduplicated: %+v", diagnostics)
	}
	if diagnostics.SymlinkFilesSkipped != 1 || diagnostics.SymlinkDirectoriesSkipped != 1 {
		t.Fatalf("symlink skip diagnostics = %+v", diagnostics)
	}
	regular, _, _, _ := index.Lookup("Go Mono")
	canonicalInside, err := filepath.EvalSymlinks(insideFont)
	if err != nil {
		t.Fatal(err)
	}
	if regular == nil || discoveryPathKey(regular.path) != discoveryPathKey(canonicalInside) {
		t.Fatalf("regular = %#v, want canonical inside target %q", regular, canonicalInside)
	}
}

func TestTTCMultiFaceIndexing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "two-face.ttc")
	if err := os.WriteFile(path, makeTestTTC(t, gomono.TTF, gomono.TTF), 0o600); err != nil {
		t.Fatal(err)
	}
	faces, examined, truncated, skipped := fontFacesBounded(path, 2)
	if skipped || examined != 2 || truncated != 0 || len(faces) != 2 {
		t.Fatalf("faces=%d examined=%d truncated=%d skipped=%v", len(faces), examined, truncated, skipped)
	}
	if faces[0].index != 0 || faces[1].index != 1 {
		t.Fatalf("face indices = %d, %d", faces[0].index, faces[1].index)
	}
	limited, examined, truncated, skipped := fontFacesBounded(path, 1)
	if skipped || examined != 1 || truncated != 1 || len(limited) != 1 {
		t.Fatalf("limited faces=%d examined=%d truncated=%d skipped=%v", len(limited), examined, truncated, skipped)
	}
}

func makeTestTTC(t *testing.T, fonts ...[]byte) []byte {
	t.Helper()
	headerLen := 12 + 4*len(fonts)
	headerLen = (headerLen + 3) &^ 3
	total := headerLen
	for _, fontData := range fonts {
		total = (total + len(fontData) + 3) &^ 3
	}
	out := make([]byte, total)
	copy(out[:4], "ttcf")
	binary.BigEndian.PutUint32(out[4:8], 0x00010000)
	binary.BigEndian.PutUint32(out[8:12], uint32(len(fonts)))
	offset := headerLen
	for i, fontData := range fonts {
		binary.BigEndian.PutUint32(out[12+i*4:16+i*4], uint32(offset))
		copy(out[offset:], fontData)
		fontCopy := out[offset : offset+len(fontData)]
		if len(fontCopy) < 12 {
			t.Fatal("invalid sfnt fixture")
		}
		numTables := int(binary.BigEndian.Uint16(fontCopy[4:6]))
		if 12+numTables*16 > len(fontCopy) {
			t.Fatal("invalid sfnt table directory")
		}
		for table := 0; table < numTables; table++ {
			field := 12 + table*16 + 8
			tableOffset := binary.BigEndian.Uint32(fontCopy[field : field+4])
			binary.BigEndian.PutUint32(fontCopy[field:field+4], tableOffset+uint32(offset))
		}
		offset = (offset + len(fontData) + 3) &^ 3
	}
	return out
}
