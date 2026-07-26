package fontglyph

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph/discovery"
)

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

func newFaceInfo(path string, index int, family, subfamily string, metadata fontdesc.FaceMetadata) faceInfo {
	return faceInfo{path: path, index: index, family: family, subfamily: subfamily, metadata: metadata}
}

func newFontIndex(faces []faceInfo) *FontIndex {
	discovered := make([]discovery.Face, len(faces))
	for i := range faces {
		discovered[i] = discovery.NewFace(faces[i].path, faces[i].index, faces[i].family, faces[i].subfamily, faces[i].metadata)
	}
	return wrapDiscoveryIndex(discovery.NewIndex(discovered))
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

func TestFontIndexCompatibilityFacadeKeepsConcreteLegacyMethodSet(t *testing.T) {
	tests := []struct {
		value      any
		name       string
		printed    string
		fieldNames []string
		fieldTypes []reflect.Type
	}{
		{faceInfo{}, "faceInfo", "fontglyph.faceInfo", []string{"path", "index", "family", "subfamily", "metadata"}, []reflect.Type{reflect.TypeOf(""), reflect.TypeOf(int(0)), reflect.TypeOf(""), reflect.TypeOf(""), reflect.TypeOf(fontdesc.FaceMetadata{})}},
		{FontIndexDiagnostics{}, "FontIndexDiagnostics", "fontglyph.FontIndexDiagnostics", []string{"Roots", "CandidateFiles", "SelectedFiles", "FilesTruncated", "FacesExamined", "FacesIndexed", "FacesTruncated", "FilesSkipped", "DuplicateFiles", "SymlinkDirectoriesSkipped", "SymlinkFilesSkipped"}, []reflect.Type{reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0))}},
		{FontResolution{}, "FontResolution", "fontglyph.FontResolution", []string{"Configured", "Found", "Regular", "Bold", "Italic", "BoldItalic", "FaceIndex", "RegularFaceIndex", "BoldFaceIndex", "ItalicFaceIndex", "BoldItalicFaceIndex"}, []reflect.Type{reflect.TypeOf(""), reflect.TypeOf(false), reflect.TypeOf(""), reflect.TypeOf(""), reflect.TypeOf(""), reflect.TypeOf(""), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0)), reflect.TypeOf(int(0))}},
	}
	for _, test := range tests {
		typeOf := reflect.TypeOf(test.value)
		if got := typeOf.Name(); got != test.name {
			t.Errorf("%T name = %q, want %q", test.value, got, test.name)
		}
		if got := fmt.Sprintf("%T", test.value); got != test.printed {
			t.Errorf("%s %%T = %q, want %q", test.name, got, test.printed)
		}
		if got := typeOf.NumMethod(); got != 0 {
			t.Errorf("%s value method count = %d, want 0", test.name, got)
		}
		if typeOf.NumField() != len(test.fieldNames) {
			t.Errorf("%s field count = %d, want %d", test.name, typeOf.NumField(), len(test.fieldNames))
			continue
		}
		for i, wantName := range test.fieldNames {
			field := typeOf.Field(i)
			if field.Name != wantName || field.Type != test.fieldTypes[i] || field.Anonymous || field.Tag != "" {
				t.Errorf("%s field %d = name %q type %v anonymous=%v tag=%q, want name %q type %v", test.name, i, field.Name, field.Type, field.Anonymous, field.Tag, wantName, test.fieldTypes[i])
			}
		}
	}
	valueType := reflect.TypeOf(FontIndex{})
	if got := fmt.Sprintf("%T", FontIndex{}); got != "fontglyph.FontIndex" {
		t.Fatalf("FontIndex %%T = %q, want fontglyph.FontIndex", got)
	}
	pointerType := reflect.PointerTo(valueType)
	methods := make([]string, pointerType.NumMethod())
	for i := range methods {
		methods[i] = pointerType.Method(i).Name
	}
	if want := []string{"Diagnostics", "Lookup"}; !reflect.DeepEqual(methods, want) {
		t.Fatalf("FontIndex method set = %v, want %v", methods, want)
	}
	if _, exposed := pointerType.MethodByName("Faces"); exposed {
		t.Fatal("FontIndex compatibility facade exposes mutable Faces authority")
	}
}

func TestFontIndexCompatibilityFacadeIsNilSafeAndDetached(t *testing.T) {
	var nilIndex *FontIndex
	if got := nilIndex.Diagnostics(); got != (FontIndexDiagnostics{}) {
		t.Fatalf("nil diagnostics = %+v", got)
	}
	regular, bold, italic, boldItalic := nilIndex.Lookup("Example Mono")
	if regular != nil || bold != nil || italic != nil || boldItalic != nil {
		t.Fatalf("nil lookup = %#v %#v %#v %#v", regular, bold, italic, boldItalic)
	}
	if faces := fontIndexFaces(nilIndex, "Example Mono"); faces != nil {
		t.Fatalf("nil private bridge = %#v", faces)
	}

	zeroIndex := &FontIndex{}
	regular, bold, italic, boldItalic = zeroIndex.Lookup("Example Mono")
	if regular != nil || bold != nil || italic != nil || boldItalic != nil || zeroIndex.Diagnostics() != (FontIndexDiagnostics{}) {
		t.Fatalf("zero index was not nil-safe: lookup=%#v %#v %#v %#v diagnostics=%+v", regular, bold, italic, boldItalic, zeroIndex.Diagnostics())
	}

	index := newFontIndex([]faceInfo{newFaceInfo("regular.ttf", 3, "Example Mono", "Regular", fontdesc.FaceMetadata{Family: "Example Mono", Subfamily: "Regular", Weight: 400})})
	regular, _, _, _ = index.Lookup("Example Mono")
	if regular == nil {
		t.Fatal("missing regular face")
	}
	regular.path = "mutated.ttf"
	regular.metadata.Weight = 900
	bridge := fontIndexFaces(index, "Example Mono")
	bridge[0].path = "bridge-mutated.ttf"
	bridge[0].metadata.Weight = 100
	fresh, _, _, _ := index.Lookup("Example Mono")
	if fresh == nil || fresh.path != "regular.ttf" || fresh.metadata.Weight != 400 {
		t.Fatalf("facade mutation changed discovery authority: %#v", fresh)
	}
}
