package raster

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"image"
	"reflect"
	"strings"
	"testing"

	"cervterm/internal/fontdesc"
	"golang.org/x/image/font/sfnt"
)

type rasterSourceField struct {
	Name     string
	Type     string
	Embedded bool
}

func TestRasterConcreteContractsAtSourceAndRuntime(t *testing.T) {
	contracts := map[string][]rasterSourceField{
		"Glyph": {
			{Name: "Image", Type: "*image.RGBA"}, {Name: "Width", Type: "int"},
			{Name: "Height", Type: "int"}, {Name: "BearingX", Type: "int"},
			{Name: "BearingY", Type: "int"}, {Name: "AdvanceX", Type: "float64"},
			{Name: "CellSpan", Type: "int"}, {Name: "HasColor", Type: "bool"},
			{Name: "Subpixel", Type: "bool"},
		},
		"ShapeGlyph": {
			{Name: "GlyphID", Type: "uint16"}, {Name: "XOffset", Type: "float64"},
			{Name: "YOffset", Type: "float64"}, {Name: "XAdvance", Type: "float64"},
		},
		"Face": {
			{Name: "Parsed", Type: "*sfnt.Font"}, {Name: "Color", Type: "*ColorFace"},
			{Name: "SourcePath", Type: "string"}, {Name: "FaceIndex", Type: "int"},
		},
		"Metrics": {
			{Name: "CellWidth", Type: "int"}, {Name: "CellHeight", Type: "int"},
			{Name: "Baseline", Type: "int"}, {Name: "PPEM", Type: "uint16"},
		},
		"FeaturePolicy": {{Name: "Features", Type: "fontdesc.FeatureSet"}},
		"ColorFace": {
			{Name: "tables", Type: "ColorTables"}, {Name: "sbix", Type: "*sbixExtractor"},
			{Name: "cbdt", Type: "*cbdtExtractor"}, {Name: "colr", Type: "*colrParser"},
			{Name: "svg", Type: "*svgExtractor"},
		},
	}
	files := map[string]string{"ColorFace": "contracts.go"}
	for name, want := range contracts {
		filename := files[name]
		if filename == "" {
			filename = "contracts.go"
		}
		if got := rasterSourceStruct(t, filename, name); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s source fields = %#v, want %#v", name, got, want)
		}
	}

	assertRasterRuntimeStruct(t, reflect.TypeOf(Glyph{}), contracts["Glyph"])
	assertRasterRuntimeStruct(t, reflect.TypeOf(ShapeGlyph{}), contracts["ShapeGlyph"])
	assertRasterRuntimeStruct(t, reflect.TypeOf(Face{}), contracts["Face"])
	assertRasterRuntimeStruct(t, reflect.TypeOf(Metrics{}), contracts["Metrics"])
	shapeType := reflect.TypeOf(ShapeGlyph{})
	if shapeType.Size() != 32 || shapeType.Align() != 8 || shapeType.Field(0).Offset != 0 || shapeType.Field(1).Offset != 8 || shapeType.Field(2).Offset != 16 || shapeType.Field(3).Offset != 24 {
		t.Fatalf("ShapeGlyph reflect layout size=%d align=%d offsets=%d/%d/%d/%d", shapeType.Size(), shapeType.Align(), shapeType.Field(0).Offset, shapeType.Field(1).Offset, shapeType.Field(2).Offset, shapeType.Field(3).Offset)
	}
	if reflect.TypeOf(uintptr(0)).Size() == 8 {
		glyphType := reflect.TypeOf(Glyph{})
		if glyphType.Size() != 64 || glyphType.Align() != 8 || glyphType.Field(0).Offset != 0 || glyphType.Field(5).Offset != 40 || glyphType.Field(8).Offset != 57 {
			t.Fatalf("Glyph 64-bit reflect layout size=%d align=%d offsets image/advance/subpixel=%d/%d/%d", glyphType.Size(), glyphType.Align(), glyphType.Field(0).Offset, glyphType.Field(5).Offset, glyphType.Field(8).Offset)
		}
		faceType := reflect.TypeOf(Face{})
		if faceType.Size() != 40 || faceType.Field(0).Offset != 0 || faceType.Field(1).Offset != 8 || faceType.Field(2).Offset != 16 || faceType.Field(3).Offset != 32 {
			t.Fatalf("Face 64-bit reflect layout size=%d offsets=%d/%d/%d/%d", faceType.Size(), faceType.Field(0).Offset, faceType.Field(1).Offset, faceType.Field(2).Offset, faceType.Field(3).Offset)
		}
	}
}

func TestRasterMethodAndFunctionSignatures(t *testing.T) {
	wantMethods := []rasterSourceField{
		{Name: "Tables", Type: "func() ColorTables"},
		{Name: "HasCOLR", Type: "func() bool"},
		{Name: "HasSVG", Type: "func() bool"},
		{Name: "Bitmap", Type: "func(glyphID uint16, ppem uint16) (BitmapGlyph, bool)"},
		{Name: "COLRGlyph", Type: "func(glyphID uint16, preferV0 bool) (COLRGlyph, error)"},
		{Name: "RasterizeSVG", Type: "func(glyphID uint16, width, height int) (*image.RGBA, bool)"},
	}
	if got := rasterSourceMethods(t, "api.go", "*ColorFace"); !reflect.DeepEqual(got, wantMethods) {
		t.Fatalf("ColorFace source methods = %#v, want %#v", got, wantMethods)
	}
	var _ func([]byte, *sfnt.Font) (*ColorFace, error) = NewColorFace
	var _ func([]byte) (ColorTables, error) = DetectColorTables
	var _ func(*sfnt.Font, rune) (uint16, bool) = GlyphIndex
	var _ func([]byte, uint16, int, int) (*image.RGBA, bool) = RasterizeSVGTableGlyph

	typ := reflect.TypeOf((*ColorFace)(nil))
	checks := map[string]struct{ in, out int }{
		"Tables": {1, 1}, "HasCOLR": {1, 1}, "HasSVG": {1, 1},
		"Bitmap": {3, 2}, "COLRGlyph": {3, 2}, "RasterizeSVG": {4, 2},
	}
	for name, want := range checks {
		method, ok := typ.MethodByName(name)
		if !ok || method.Type.NumIn() != want.in || method.Type.NumOut() != want.out {
			t.Fatalf("%s signature = %v", name, method.Type)
		}
	}
	bitmap, _ := typ.MethodByName("Bitmap")
	if bitmap.Type.In(1).Kind() != reflect.Uint16 || bitmap.Type.In(2).Kind() != reflect.Uint16 || bitmap.Type.Out(0) != reflect.TypeOf(BitmapGlyph{}) || bitmap.Type.Out(1).Kind() != reflect.Bool {
		t.Fatalf("Bitmap full signature = %v", bitmap.Type)
	}
	colr, _ := typ.MethodByName("COLRGlyph")
	if colr.Type.In(1).Kind() != reflect.Uint16 || colr.Type.In(2).Kind() != reflect.Bool || colr.Type.Out(0) != reflect.TypeOf(COLRGlyph{}) || colr.Type.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("COLRGlyph full signature = %v", colr.Type)
	}
}

func rasterSourceStruct(t *testing.T, filename, typeName string) []rasterSourceField {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			if typeSpec.Name.Name != typeName {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("%s is %T, want struct", typeName, typeSpec.Type)
			}
			result := make([]rasterSourceField, 0, len(structure.Fields.List))
			for _, field := range structure.Fields.List {
				var rendered bytes.Buffer
				if err := format.Node(&rendered, token.NewFileSet(), field.Type); err != nil {
					t.Fatal(err)
				}
				if len(field.Names) == 0 {
					result = append(result, rasterSourceField{Type: rendered.String(), Embedded: true})
					continue
				}
				for _, name := range field.Names {
					result = append(result, rasterSourceField{Name: name.Name, Type: rendered.String()})
				}
			}
			return result
		}
	}
	t.Fatalf("type %s not found in %s", typeName, filename)
	return nil
}

func rasterSourceMethods(t *testing.T, filename, receiver string) []rasterSourceField {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	var result []rasterSourceField
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
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
		result = append(result, rasterSourceField{Name: function.Name.Name, Type: renderedType.String()})
	}
	return result
}

func assertRasterRuntimeStruct(t *testing.T, typ reflect.Type, want []rasterSourceField) {
	t.Helper()
	if typ.NumField() != len(want) {
		t.Fatalf("%s field count = %d, want %d", typ, typ.NumField(), len(want))
	}
	for index, expected := range want {
		field := typ.Field(index)
		runtimeType := strings.ReplaceAll(field.Type.String(), "raster.", "")
		if field.Name != expected.Name || runtimeType != expected.Type || field.Anonymous != expected.Embedded {
			t.Fatalf("%s field %d = %s %s embedded=%t, want %#v", typ, index, field.Name, field.Type, field.Anonymous, expected)
		}
	}
}

var _ = fontdesc.FeatureSet{}
