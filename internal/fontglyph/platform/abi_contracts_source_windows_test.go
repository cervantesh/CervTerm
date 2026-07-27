//go:build windows

package platform

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

func TestDirectWriteABIStructsAtSourceAndRuntime(t *testing.T) {
	contracts := []struct {
		file string
		name string
		want []sourceContractField
	}{
		{"directwrite_bridge_windows.go", "dwriteScriptAnalysis", []sourceContractField{{Name: "Script", Type: "uint16"}, {Name: "_", Type: "uint16"}, {Name: "Shapes", Type: "uint32"}}},
		{"directwrite_bridge_windows.go", "dwriteGlyphOffset", []sourceContractField{{Name: "AdvanceOffset", Type: "float32"}, {Name: "AscenderOffset", Type: "float32"}}},
		{"directwrite_bridge_windows.go", "dwriteFontFeature", []sourceContractField{{Name: "Name", Type: "uint32"}, {Name: "Parameter", Type: "uint32"}}},
		{"directwrite_bridge_windows.go", "dwriteTypographicFeatures", []sourceContractField{{Name: "Features", Type: "*dwriteFontFeature"}, {Name: "FeatureCount", Type: "uint32"}}},
		{"directwrite_raster_windows.go", "dwriteGlyphRun", []sourceContractField{
			{Name: "FontFace", Type: "*iUnknown"}, {Name: "FontEmSize", Type: "float32"},
			{Name: "GlyphCount", Type: "uint32"}, {Name: "GlyphIndices", Type: "*uint16"},
			{Name: "GlyphAdvances", Type: "*float32"}, {Name: "GlyphOffsets", Type: "*dwriteGlyphOffset"},
			{Name: "IsSideways", Type: "int32"}, {Name: "BidiLevel", Type: "uint32"},
		}},
		{"directwrite_raster_windows.go", "dwriteRect", []sourceContractField{{Name: "Left", Type: "int32"}, {Name: "Top", Type: "int32"}, {Name: "Right", Type: "int32"}, {Name: "Bottom", Type: "int32"}}},
		{"directwrite_analysis_windows.go", "dwriteTextAnalysisSource", []sourceContractField{{Name: "lpVtbl", Type: "*dwriteTextAnalysisSourceVtbl"}, {Name: "owner", Type: "*dwriteTextAnalysis"}}},
		{"directwrite_analysis_windows.go", "dwriteTextAnalysisSink", []sourceContractField{{Name: "lpVtbl", Type: "*dwriteTextAnalysisSinkVtbl"}, {Name: "owner", Type: "*dwriteTextAnalysis"}}},
	}
	for _, contract := range contracts {
		if got := sourceStructContract(t, contract.file, contract.name); !reflect.DeepEqual(got, contract.want) {
			t.Fatalf("%s source fields = %#v, want %#v", contract.name, got, contract.want)
		}
		for index, field := range sourceStructContract(t, contract.file, contract.name) {
			if field.Embedded {
				t.Fatalf("%s field %d is unexpectedly embedded: %#v", contract.name, index, field)
			}
		}
	}

	assertUintptrVTable(t, "directwrite_bridge_windows.go", "iUnknownVtbl", []string{"queryInterface", "addRef", "release"})
	assertUintptrVTable(t, "directwrite_bridge_windows.go", "iWriteTextAnalyzerVtbl", []string{"queryInterface", "addRef", "release", "analyzeScript", "analyzeBidi", "analyzeNumberSubstitution", "analyzeLineBreakpoints", "getGlyphs", "getGlyphPlacements", "getGdiCompatibleGlyphPlacements"})
	assertUintptrVTable(t, "directwrite_bridge_windows.go", "iWriteFactoryVtbl", []string{"queryInterface", "addRef", "release", "getSystemFontCollection", "createCustomFontCollection", "registerFontCollectionLoader", "unregisterFontCollectionLoader", "createFontFileReference", "createCustomFontFileReference", "createFontFace", "createRenderingParams", "createMonitorRenderingParams", "createCustomRenderingParams", "registerFontFileLoader", "unregisterFontFileLoader", "createTextFormat", "createTypography", "getGdiInterop", "createTextLayout", "createGdiCompatibleTextLayout", "createEllipsisTrimmingSign", "createTextAnalyzer", "createNumberSubstitution", "createGlyphRunAnalysis"})
	assertUintptrVTable(t, "directwrite_analysis_windows.go", "dwriteTextAnalysisSourceVtbl", []string{"queryInterface", "addRef", "release", "getTextAtPosition", "getTextBeforePosition", "getParagraphDirection", "getLocaleName", "getNumberSubstitution"})
	assertUintptrVTable(t, "directwrite_analysis_windows.go", "dwriteTextAnalysisSinkVtbl", []string{"queryInterface", "addRef", "release", "setScriptAnalysis", "setLineBreakpoints", "setBidiLevel", "setNumberSubstitution"})
	assertUintptrVTable(t, "directwrite_raster_windows.go", "dwriteFontFileVtbl", []string{"queryInterface", "addRef", "release", "getReferenceKey", "getLoader", "analyze"})
	assertUintptrVTable(t, "directwrite_raster_windows.go", "dwriteGlyphRunAnalysisVtbl", []string{"queryInterface", "addRef", "release", "getAlphaTextureBounds", "createAlphaTexture", "getAlphaBlendParams"})

	assertDirectWriteReflectLayout(t, reflect.TypeOf(dwriteScriptAnalysis{}), 8, 4, []uintptr{0, 2, 4})
	assertDirectWriteReflectLayout(t, reflect.TypeOf(dwriteGlyphOffset{}), 8, 4, []uintptr{0, 4})
	assertDirectWriteReflectLayout(t, reflect.TypeOf(dwriteRect{}), 16, 4, []uintptr{0, 4, 8, 12})
	if reflect.TypeOf(uintptr(0)).Size() == 8 {
		assertDirectWriteReflectLayout(t, reflect.TypeOf(dwriteGlyphRun{}), 48, 8, []uintptr{0, 8, 12, 16, 24, 32, 40, 44})
	}
}

func TestDirectWriteNativeMethodSignaturesAtSource(t *testing.T) {
	checks := []struct {
		file     string
		receiver string
		name     string
		want     string
	}{
		{"directwrite_bridge_windows.go", "*iWriteFactory", "createTextAnalyzer", "func() (*iWriteTextAnalyzer, error)"},
		{"directwrite_bridge_windows.go", "*iWriteTextAnalyzer", "shapeText", "func(text string, fontFace *iUnknown, ppem uint16, features fontdesc.FeatureSet) ([]ShapeGlyph, bool, error)"},
		{"directwrite_bridge_windows.go", "*iWriteFactory", "createFontFaceFromPathIndex", "func(path string, faceIndex int) (*iUnknown, error)"},
		{"directwrite_analysis_windows.go", "*iWriteTextAnalyzer", "analyzeScript", "func(text []uint16) (dwriteScriptAnalysis, bool, error)"},
		{"directwrite_raster_windows.go", "*iWriteFactory", "openFontFile", "func(path string) (*dwriteFontFile, uint32, uint32, error)"},
		{"directwrite_raster_windows.go", "*iWriteFactory", "createAnalyzedFontFace", "func(file *dwriteFontFile, faceType uint32, faceIndex int) (*iUnknown, error)"},
		{"directwrite_raster_windows.go", "*dwriteRasterizer", "RasterizeGlyph", "func(glyphID uint16, cellW, cellH, baseline, cellSpan int, advancePx float32) (*image.RGBA, bool)"},
		{"directwrite_raster_windows.go", "*dwriteRasterizer", "Close", "func()"},
	}
	for _, check := range checks {
		if got := directWriteSourceMethod(t, check.file, check.receiver, check.name); got != check.want {
			t.Fatalf("%s.%s source signature = %q, want %q", check.receiver, check.name, got, check.want)
		}
	}
}

func assertDirectWriteReflectLayout(t *testing.T, typ reflect.Type, size uintptr, align int, offsets []uintptr) {
	t.Helper()
	if typ.Size() != size || typ.Align() != align || typ.NumField() != len(offsets) {
		t.Fatalf("%s reflect layout size=%d align=%d fields=%d, want %d/%d/%d", typ, typ.Size(), typ.Align(), typ.NumField(), size, align, len(offsets))
	}
	for index, offset := range offsets {
		if typ.Field(index).Offset != offset {
			t.Fatalf("%s reflect field %d offset=%d, want %d", typ, index, typ.Field(index).Offset, offset)
		}
	}
}

func assertUintptrVTable(t *testing.T, filename, typeName string, names []string) {
	t.Helper()
	want := make([]sourceContractField, len(names))
	for index, name := range names {
		want[index] = sourceContractField{Name: name, Type: "uintptr"}
	}
	if got := sourceStructContract(t, filename, typeName); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s fields = %#v, want %#v", typeName, got, want)
	}
}

func directWriteSourceMethod(t *testing.T, filename, receiver, name string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != name || function.Recv == nil || len(function.Recv.List) != 1 {
			continue
		}
		var renderedReceiver bytes.Buffer
		if err := format.Node(&renderedReceiver, token.NewFileSet(), function.Recv.List[0].Type); err != nil {
			t.Fatal(err)
		}
		if renderedReceiver.String() != receiver {
			continue
		}
		var renderedType bytes.Buffer
		if err := format.Node(&renderedType, token.NewFileSet(), function.Type); err != nil {
			t.Fatal(err)
		}
		return renderedType.String()
	}
	t.Fatalf("method %s.%s not found in %s", receiver, name, filename)
	return ""
}
