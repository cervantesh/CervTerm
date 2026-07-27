package fontglyph

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"cervterm/internal/fontdesc"
)

type rootSourceField struct {
	Name     string
	Type     string
	Embedded bool
}

func TestRootShapingContractsAtSourceAndRuntime(t *testing.T) {
	wantGlyph := []rootSourceField{
		{Name: "GlyphID", Type: "uint16"},
		{Name: "XOffset", Type: "float64"},
		{Name: "YOffset", Type: "float64"},
		{Name: "XAdvance", Type: "float64"},
	}
	if got := rootSourceStructOrInterface(t, "shaper.go", "ShapedGlyph", false); !reflect.DeepEqual(got, wantGlyph) {
		t.Fatalf("ShapedGlyph source fields = %#v, want %#v", got, wantGlyph)
	}
	wantShaper := []rootSourceField{{Name: "Shape", Type: "func(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool)"}}
	if got := rootSourceStructOrInterface(t, "shaper.go", "Shaper", true); !reflect.DeepEqual(got, wantShaper) {
		t.Fatalf("Shaper source methods = %#v, want %#v", got, wantShaper)
	}
	wantFeatureShaper := []rootSourceField{{Name: "ShapeFeatures", Type: "func(cluster string, face loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool)"}}
	if got := rootSourceStructOrInterface(t, "shaper.go", "FeatureShaper", true); !reflect.DeepEqual(got, wantFeatureShaper) {
		t.Fatalf("FeatureShaper source methods = %#v, want %#v", got, wantFeatureShaper)
	}

	typ := reflect.TypeOf(ShapedGlyph{})
	if typ.PkgPath() != "cervterm/internal/fontglyph" || typ.Name() != "ShapedGlyph" || typ.NumField() != len(wantGlyph) {
		t.Fatalf("root glyph identity = %s.%s fields=%d", typ.PkgPath(), typ.Name(), typ.NumField())
	}
	for index, expected := range wantGlyph {
		field := typ.Field(index)
		if field.Name != expected.Name || field.Type.String() != expected.Type || field.Anonymous {
			t.Fatalf("ShapedGlyph field %d = %s %s embedded=%t, want %#v", index, field.Name, field.Type, field.Anonymous, expected)
		}
	}
	if typ.Size() != 32 || typ.Align() != 8 || typ.Field(0).Offset != 0 || typ.Field(1).Offset != 8 || typ.Field(2).Offset != 16 || typ.Field(3).Offset != 24 {
		t.Fatalf("ShapedGlyph reflect layout size=%d align=%d offsets=%d/%d/%d/%d", typ.Size(), typ.Align(), typ.Field(0).Offset, typ.Field(1).Offset, typ.Field(2).Offset, typ.Field(3).Offset)
	}
	shaperMethod, ok := reflect.TypeOf((*Shaper)(nil)).Elem().MethodByName("Shape")
	if !ok || shaperMethod.Type.NumIn() != 3 || shaperMethod.Type.In(0).Kind() != reflect.String || shaperMethod.Type.In(1) != reflect.TypeOf(loadedFace{}) || shaperMethod.Type.In(2).Kind() != reflect.Uint16 || shaperMethod.Type.NumOut() != 2 || shaperMethod.Type.Out(0) != reflect.TypeOf([]ShapedGlyph{}) || shaperMethod.Type.Out(1).Kind() != reflect.Bool {
		t.Fatalf("Shaper.Shape runtime signature = %v", shaperMethod.Type)
	}
	featureMethod, ok := reflect.TypeOf((*FeatureShaper)(nil)).Elem().MethodByName("ShapeFeatures")
	if !ok || featureMethod.Type.NumIn() != 4 || featureMethod.Type.In(0).Kind() != reflect.String || featureMethod.Type.In(1) != reflect.TypeOf(loadedFace{}) || featureMethod.Type.In(2).Kind() != reflect.Uint16 || featureMethod.Type.In(3) != reflect.TypeOf(fontdesc.FeatureSet{}) || featureMethod.Type.NumOut() != 2 || featureMethod.Type.Out(0) != reflect.TypeOf([]ShapedGlyph{}) || featureMethod.Type.Out(1).Kind() != reflect.Bool {
		t.Fatalf("FeatureShaper.ShapeFeatures runtime signature = %v", featureMethod.Type)
	}
}

func rootSourceStructOrInterface(t *testing.T, filename, typeName string, wantInterface bool) []rootSourceField {
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
			var fields *ast.FieldList
			if wantInterface {
				iface, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					t.Fatalf("%s is %T, want interface", typeName, typeSpec.Type)
				}
				fields = iface.Methods
			} else {
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					t.Fatalf("%s is %T, want struct", typeName, typeSpec.Type)
				}
				fields = structure.Fields
			}
			result := make([]rootSourceField, 0, len(fields.List))
			for _, field := range fields.List {
				var rendered bytes.Buffer
				if err := format.Node(&rendered, token.NewFileSet(), field.Type); err != nil {
					t.Fatal(err)
				}
				if len(field.Names) == 0 {
					result = append(result, rootSourceField{Type: rendered.String(), Embedded: true})
					continue
				}
				for _, name := range field.Names {
					result = append(result, rootSourceField{Name: name.Name, Type: rendered.String()})
				}
			}
			return result
		}
	}
	t.Fatalf("type %s not found in %s", typeName, filename)
	return nil
}
