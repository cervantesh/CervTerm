package shape

import (
	"reflect"
	"testing"

	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph/internal/face"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/sfnt"
)

func TestAdditiveContractsAreConcreteDetachedValues(t *testing.T) {
	glyph := reflect.TypeOf(Glyph{})
	if glyph.NumField() != 4 || glyph.NumMethod() != 0 {
		t.Fatalf("Glyph fields/methods = %d/%d", glyph.NumField(), glyph.NumMethod())
	}
	plan := reflect.TypeOf(Plan{})
	if plan.NumField() != 8 || plan.NumMethod() != 0 {
		t.Fatalf("Plan fields/methods = %d/%d", plan.NumField(), plan.NumMethod())
	}
	source := []SourceFace{{Source: "test:one", Metadata: fontdesc.FaceMetadata{Family: "One"}}}
	snapshot := append([]SourceFace(nil), source...)
	resolver := NewResolver(func(string) []SourceFace { return append([]SourceFace(nil), snapshot...) })
	source[0].Source = "mutated"
	got := resolver.lookup("One")
	if got[0].Source != "test:one" {
		t.Fatalf("detached lookup changed to %q", got[0].Source)
	}
}

func TestFaceRefExposesOnlyShapingProjection(t *testing.T) {
	parsed, err := sfnt.Parse(gomono.TTF)
	if err != nil {
		t.Fatal(err)
	}
	ref := face.NewRef(parsed, "embedded:gomono", 0)
	if !ref.Valid() {
		t.Fatal("valid parsed face reported invalid")
	}
	if source, index, ok := ref.Source(); !ok || source != "embedded:gomono" || index != 0 {
		t.Fatalf("source projection = %q#%d, %v", source, index, ok)
	}
	var buffer sfnt.Buffer
	glyph, err := ref.GlyphIndex(&buffer, 'A')
	if err != nil || glyph == 0 {
		t.Fatalf("glyph projection = %d, %v", glyph, err)
	}
}
