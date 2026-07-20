package fontglyph

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"cervterm/internal/fontdesc"

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

func TestGoMonoFaceMetadataFromOS2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "GoMono.ttf")
	if err := os.WriteFile(path, gomono.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	faces := fontFaces(path)
	if len(faces) != 1 {
		t.Fatalf("fontFaces() returned %d faces", len(faces))
	}
	metadata := faces[0].metadata
	if metadata.Family != "Go Mono" || metadata.Weight != 400 || metadata.Stretch != 100 || metadata.Style != fontdesc.StyleNormal || metadata.CollectionIndex != 0 {
		t.Fatalf("Go Mono metadata = %+v", metadata)
	}
}

func TestOS2NumericMetadataAndDefaults(t *testing.T) {
	tests := []struct {
		name                     string
		weight, width, selection uint16
		wantWeight, wantStretch  int
		wantStyle                fontdesc.Style
	}{
		{name: "italic condensed", weight: 200, width: 3, selection: 1, wantWeight: 200, wantStretch: 75, wantStyle: fontdesc.StyleItalic},
		{name: "oblique expanded", weight: 800, width: 8, selection: 1 << 9, wantWeight: 800, wantStretch: 150, wantStyle: fontdesc.StyleOblique},
		{name: "invalid defaults", weight: 99, width: 10, selection: 0, wantWeight: 400, wantStretch: 100, wantStyle: fontdesc.StyleNormal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "patched.ttf")
			if err := os.WriteFile(path, patchTestOS2(t, gomono.TTF, tt.weight, tt.width, tt.selection), 0o600); err != nil {
				t.Fatal(err)
			}
			faces := fontFaces(path)
			if len(faces) != 1 {
				t.Fatalf("fontFaces() returned %d faces", len(faces))
			}
			got := faces[0].metadata
			if got.Weight != tt.wantWeight || got.Stretch != tt.wantStretch || got.Style != tt.wantStyle {
				t.Fatalf("metadata = %+v, want weight=%d stretch=%d style=%s", got, tt.wantWeight, tt.wantStretch, tt.wantStyle)
			}
		})
	}
}

func TestOS2ReservedObliqueBitIgnoredBeforeVersion4(t *testing.T) {
	data := patchTestOS2(t, gomono.TTF, 400, 5, 1<<9)
	record := testTableRecord(t, data, "OS/2")
	offset := int(binary.BigEndian.Uint32(data[record+8 : record+12]))
	binary.BigEndian.PutUint16(data[offset:offset+2], 3)
	path := filepath.Join(t.TempDir(), "old-os2.ttf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	faces := fontFaces(path)
	if len(faces) != 1 || faces[0].metadata.Style != fontdesc.StyleNormal {
		t.Fatalf("old OS/2 reserved bit metadata = %+v", faces)
	}
}

func TestTTCFacesHaveIndependentOS2Metadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "independent.ttc")
	light := patchTestOS2(t, gomono.TTF, 200, 2, 0)
	oblique := patchTestOS2(t, gomono.TTF, 800, 9, 1<<9)
	if err := os.WriteFile(path, makeTestTTC(t, light, oblique), 0o600); err != nil {
		t.Fatal(err)
	}
	faces, examined, truncated, skipped := fontFacesBounded(path, 2)
	if skipped || examined != 2 || truncated != 0 || len(faces) != 2 {
		t.Fatalf("faces=%d examined=%d truncated=%d skipped=%v", len(faces), examined, truncated, skipped)
	}
	if faces[0].index != 0 || faces[0].metadata.CollectionIndex != 0 || faces[0].metadata.Weight != 200 || faces[0].metadata.Stretch != 62 || faces[0].metadata.Style != fontdesc.StyleNormal {
		t.Fatalf("face 0 = %+v", faces[0])
	}
	if faces[1].index != 1 || faces[1].metadata.CollectionIndex != 1 || faces[1].metadata.Weight != 800 || faces[1].metadata.Stretch != 200 || faces[1].metadata.Style != fontdesc.StyleOblique {
		t.Fatalf("face 1 = %+v", faces[1])
	}
}

func TestCorruptOS2BoundsSkipOnlyFace(t *testing.T) {
	data := append([]byte(nil), gomono.TTF...)
	record := testTableRecord(t, data, "OS/2")
	binary.BigEndian.PutUint32(data[record+8:record+12], uint32(len(data)+1))
	path := filepath.Join(t.TempDir(), "corrupt.ttf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	faces, examined, truncated, skipped := fontFacesBounded(path, 1)
	if skipped || examined != 1 || truncated != 0 || len(faces) != 0 {
		t.Fatalf("corrupt metadata policy: faces=%d examined=%d truncated=%d skipped=%v", len(faces), examined, truncated, skipped)
	}
	index := BuildFontIndex([]string{filepath.Dir(path)})
	diagnostics := index.Diagnostics()
	if diagnostics.FacesExamined != 1 || diagnostics.FacesIndexed != 0 || diagnostics.FilesSkipped != 0 {
		t.Fatalf("corrupt metadata diagnostics = %+v", diagnostics)
	}
}

func patchTestOS2(t *testing.T, source []byte, weight, width, selection uint16) []byte {
	t.Helper()
	data := append([]byte(nil), source...)
	record := testTableRecord(t, data, "OS/2")
	offset := int(binary.BigEndian.Uint32(data[record+8 : record+12]))
	length := int(binary.BigEndian.Uint32(data[record+12 : record+16]))
	if offset < 0 || length < 64 || offset > len(data)-length {
		t.Fatal("invalid OS/2 fixture bounds")
	}
	if selection&(1<<9) != 0 {
		binary.BigEndian.PutUint16(data[offset:offset+2], 4)
	}
	binary.BigEndian.PutUint16(data[offset+4:offset+6], weight)
	binary.BigEndian.PutUint16(data[offset+6:offset+8], width)
	binary.BigEndian.PutUint16(data[offset+62:offset+64], selection)
	return data
}

func testTableRecord(t *testing.T, data []byte, tag string) int {
	t.Helper()
	if len(data) < 12 {
		t.Fatal("invalid sfnt fixture")
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if numTables > maxSFNTTableRecords || 12+numTables*16 > len(data) {
		t.Fatal("invalid sfnt table directory")
	}
	for i := 0; i < numTables; i++ {
		record := 12 + i*16
		if string(data[record:record+4]) == tag {
			return record
		}
	}
	t.Fatalf("table %q not found", tag)
	return 0
}
