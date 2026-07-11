package fontglyph

import (
	"path/filepath"
	"testing"
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
			{path: "regular.ttf", family: "Example Mono", subfamily: "Book"},
			{path: "bold.ttf", family: "Example Mono", subfamily: "Bold"},
			{path: "italic.ttf", family: "Example Mono", subfamily: "Oblique"},
			{path: "bold-italic.ttf", family: "Example Mono", subfamily: "Italic Bold"},
		},
	}}
	regular, bold, italic, boldItalic := index.Lookup(" EXAMPLE   MONO ")
	if regular == nil || regular.path != "regular.ttf" || bold == nil || bold.path != "bold.ttf" || italic == nil || italic.path != "italic.ttf" || boldItalic == nil || boldItalic.path != "bold-italic.ttf" {
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
}

func TestTTCMultiFaceIndexing(t *testing.T) {
	t.Skip("no redistributable TTC fixture is present")
}
