package fontglyph

import (
	"reflect"
	"testing"
)

type rootABIContract struct {
	value   any
	size    uintptr
	align   int
	offsets []uintptr
}

func TestRootConcreteABI(t *testing.T) {
	contracts := []rootABIContract{
		{RasterizedGlyph{}, 64, 8, []uintptr{0, 8, 16, 24, 32, 40, 48, 56, 57}},
		{OpenTypeBackend{}, 216, 8, []uintptr{0, 24, 72, 73, 80, 88, 96, 104, 112, 128, 184, 200, 204}},
		{ColorTables{}, 10, 2, []uintptr{0, 1, 2, 3, 4, 5, 6, 8}},
		{COLRGlyph{}, 32, 8, []uintptr{0, 8}},
		{COLRLayer{}, 328, 8, []uintptr{0, 2, 4, 8, 16, 64, 72, 144, 216, 272, 280, 304}},
		{ShapedGlyph{}, 32, 8, []uintptr{0, 8, 16, 24}},
	}
	for _, contract := range contracts {
		typ := reflect.TypeOf(contract.value)
		if typ.Size() != contract.size || typ.Align() != contract.align || typ.NumField() != len(contract.offsets) {
			t.Fatalf("%s ABI size=%d align=%d fields=%d, want %d/%d/%d", typ.Name(), typ.Size(), typ.Align(), typ.NumField(), contract.size, contract.align, len(contract.offsets))
		}
		for index, offset := range contract.offsets {
			if typ.Field(index).Offset != offset {
				t.Fatalf("%s field %d offset=%d, want %d", typ.Name(), index, typ.Field(index).Offset, offset)
			}
		}
	}
}

func TestRootExportedMethodSets(t *testing.T) {
	contracts := []struct {
		typeOf any
		want   []string
	}{
		{(*OpenTypeBackend)(nil), []string{"CellMetrics", "Close", "InspectClusterGlyph", "Rasterize", "RasterizeCluster", "RasterizeRun", "SetShaper", "SupportsLigatures", "TextRasterEngine"}},
		{ColorTables{}, []string{"HasAnyColor", "HasBitmapColor", "HasLayerColor", "HasRenderableLayerColor", "PreferredFormat"}},
		{RasterizedGlyph{}, nil},
		{COLRGlyph{}, nil},
		{COLRLayer{}, nil},
		{ShapedGlyph{}, nil},
	}
	for _, contract := range contracts {
		typ := reflect.TypeOf(contract.typeOf)
		var got []string
		for index := 0; index < typ.NumMethod(); index++ {
			got = append(got, typ.Method(index).Name)
		}
		if !reflect.DeepEqual(got, contract.want) {
			t.Fatalf("%s exported methods=%v, want %v", typ, got, contract.want)
		}
	}
}
