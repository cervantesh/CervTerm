package platform

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"image"
	"reflect"
	"testing"
)

type sourceContractField struct {
	Name     string
	Type     string
	Embedded bool
}

func TestPlatformConcreteContractsAtSourceAndRuntime(t *testing.T) {
	structs := map[string][]sourceContractField{
		"ShapeGlyph": {
			{Name: "GlyphID", Type: "uint16"},
			{Name: "XOffset", Type: "float64"},
			{Name: "YOffset", Type: "float64"},
			{Name: "XAdvance", Type: "float64"},
		},
		"FaceSource": {
			{Name: "Path", Type: "string"},
			{Name: "Index", Type: "int"},
		},
		"TextRasterSpec": {
			{Name: "Mode", Type: "string"},
			{Name: "Source", Type: "FaceSource"},
			{Name: "SizePoints", Type: "float64"},
			{Name: "DPI", Type: "float64"},
		},
		"ShapeRequest": {
			{Name: "Text", Type: "string"},
			{Name: "Face", Type: "FaceSource"},
			{Name: "PPEM", Type: "uint16"},
			{Name: "Features", Type: "fontdesc.FeatureSet"},
		},
		"NativeHooks": {
			{Name: "Shape", Type: "func(ShapeRequest) ([]ShapeGlyph, bool)"},
			{Name: "Raster", Type: "func(TextRasterSpec) GlyphRasterizer"},
		},
	}
	for name, want := range structs {
		if got := sourceStructContract(t, "contracts.go", name); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s source fields = %#v, want %#v", name, got, want)
		}
	}
	wantInterface := []sourceContractField{
		{Name: "RasterizeGlyph", Type: "func(glyphID uint16, cellW, cellH, baseline, cellSpan int, advancePx float32) (*image.RGBA, bool)"},
		{Name: "Close", Type: "func()"},
	}
	if got := sourceInterfaceContract(t, "contracts.go", "GlyphRasterizer"); !reflect.DeepEqual(got, wantInterface) {
		t.Fatalf("GlyphRasterizer source methods = %#v, want %#v", got, wantInterface)
	}

	glyph := reflect.TypeOf(ShapeGlyph{})
	assertRuntimeStruct(t, glyph, []sourceContractField{
		{Name: "GlyphID", Type: "uint16"}, {Name: "XOffset", Type: "float64"},
		{Name: "YOffset", Type: "float64"}, {Name: "XAdvance", Type: "float64"},
	})
	if glyph.Size() != 32 || glyph.Align() != 8 || glyph.Field(0).Offset != 0 || glyph.Field(1).Offset != 8 || glyph.Field(2).Offset != 16 || glyph.Field(3).Offset != 24 {
		t.Fatalf("ShapeGlyph reflect layout size=%d align=%d offsets=%d/%d/%d/%d", glyph.Size(), glyph.Align(), glyph.Field(0).Offset, glyph.Field(1).Offset, glyph.Field(2).Offset, glyph.Field(3).Offset)
	}
	iface := reflect.TypeOf((*GlyphRasterizer)(nil)).Elem()
	method, ok := iface.MethodByName("RasterizeGlyph")
	if !ok || method.Type.NumIn() != 6 || method.Type.In(0) != reflect.TypeOf(uint16(0)) || method.Type.In(5) != reflect.TypeOf(float32(0)) || method.Type.NumOut() != 2 || method.Type.Out(0) != reflect.TypeOf((*image.RGBA)(nil)) || method.Type.Out(1).Kind() != reflect.Bool {
		t.Fatalf("RasterizeGlyph runtime signature = %v", method.Type)
	}
	closeMethod, ok := iface.MethodByName("Close")
	if !ok || closeMethod.Type.NumIn() != 0 || closeMethod.Type.NumOut() != 0 {
		t.Fatalf("Close runtime signature = %v", closeMethod.Type)
	}
}

func TestPlatformFunctionSignaturesCompile(t *testing.T) {
	var _ func(ShapeRequest) ([]ShapeGlyph, bool) = Shape
	var _ func(ShapeRequest, ShapeAllocator[ShapeGlyph], ShapeAssigner[ShapeGlyph]) ([]ShapeGlyph, bool) = ShapeInto[ShapeGlyph]
	var _ func(TextRasterSpec) GlyphRasterizer = NewTextRasterizer
	var _ func(FaceSource, float64, float64) (GlyphRasterizer, error) = NewDirectWriteRasterizer
}

func sourceStructContract(t *testing.T, filename, typeName string) []sourceContractField {
	t.Helper()
	typeSpec := sourceTypeSpec(t, filename, typeName)
	structure, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		t.Fatalf("%s is %T, want struct", typeName, typeSpec.Type)
	}
	return sourceFieldList(t, structure.Fields)
}

func sourceInterfaceContract(t *testing.T, filename, typeName string) []sourceContractField {
	t.Helper()
	typeSpec := sourceTypeSpec(t, filename, typeName)
	iface, ok := typeSpec.Type.(*ast.InterfaceType)
	if !ok {
		t.Fatalf("%s is %T, want interface", typeName, typeSpec.Type)
	}
	return sourceFieldList(t, iface.Methods)
}

func sourceTypeSpec(t *testing.T, filename, typeName string) *ast.TypeSpec {
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
			if typeSpec.Name.Name == typeName {
				return typeSpec
			}
		}
	}
	t.Fatalf("type %s not found in %s", typeName, filename)
	return nil
}

func sourceFieldList(t *testing.T, fields *ast.FieldList) []sourceContractField {
	t.Helper()
	result := make([]sourceContractField, 0, len(fields.List))
	for _, field := range fields.List {
		var rendered bytes.Buffer
		if err := format.Node(&rendered, token.NewFileSet(), field.Type); err != nil {
			t.Fatal(err)
		}
		if len(field.Names) == 0 {
			result = append(result, sourceContractField{Type: rendered.String(), Embedded: true})
			continue
		}
		for _, name := range field.Names {
			result = append(result, sourceContractField{Name: name.Name, Type: rendered.String()})
		}
	}
	return result
}

func assertRuntimeStruct(t *testing.T, typ reflect.Type, want []sourceContractField) {
	t.Helper()
	if typ.NumField() != len(want) {
		t.Fatalf("%s field count = %d, want %d", typ, typ.NumField(), len(want))
	}
	for index, expected := range want {
		field := typ.Field(index)
		if field.Name != expected.Name || field.Type.String() != expected.Type || field.Anonymous != expected.Embedded {
			t.Fatalf("%s field %d = %s %s embedded=%t, want %#v", typ, index, field.Name, field.Type, field.Anonymous, expected)
		}
	}
}
