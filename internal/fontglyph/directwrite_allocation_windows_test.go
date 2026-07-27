//go:build windows

package fontglyph

import (
	"reflect"
	"testing"

	"cervterm/internal/fontdesc"
	platformpkg "cervterm/internal/fontglyph/platform"
)

var directWriteShapingAllocationSink []ShapedGlyph

func TestDirectWriteOneResultAllocationMatchesBaselineOutput(t *testing.T) {
	request := directWriteAllocationRequest(t)
	baseline, baselineOK := directWriteTwoResultBaseline(request)
	candidate, candidateOK := shapeWithDirectWrite(request.Text, request.Face.Path, request.Face.Index, request.PPEM, request.Features)
	if baselineOK != candidateOK || !reflect.DeepEqual(candidate, baseline) {
		t.Fatalf("one-result shape = %#v/%t, baseline = %#v/%t", candidate, candidateOK, baseline, baselineOK)
	}
	for index := range candidate {
		if candidate[index].GlyphID != baseline[index].GlyphID || candidate[index].XAdvance != baseline[index].XAdvance || candidate[index].XOffset != baseline[index].XOffset || candidate[index].YOffset != baseline[index].YOffset {
			t.Fatalf("glyph %d changed: candidate=%#v baseline=%#v", index, candidate[index], baseline[index])
		}
	}
}

func TestDirectWriteOneResultAllocationEliminatesConversionHeapObject(t *testing.T) {
	request := directWriteAllocationRequest(t)
	if shaped, ok := directWriteTwoResultBaseline(request); !ok || len(shaped) == 0 {
		t.Fatal("baseline DirectWrite shape failed")
	}
	if shaped, ok := shapeWithDirectWrite(request.Text, request.Face.Path, request.Face.Index, request.PPEM, request.Features); !ok || len(shaped) == 0 {
		t.Fatal("one-result DirectWrite shape failed")
	}
	baselineAllocs := testing.AllocsPerRun(5, func() {
		shaped, ok := directWriteTwoResultBaseline(request)
		if !ok {
			panic("baseline DirectWrite shape failed")
		}
		directWriteShapingAllocationSink = shaped
	})
	candidateAllocs := testing.AllocsPerRun(5, func() {
		shaped, ok := shapeWithDirectWrite(request.Text, request.Face.Path, request.Face.Index, request.PPEM, request.Features)
		if !ok {
			panic("one-result DirectWrite shape failed")
		}
		directWriteShapingAllocationSink = shaped
	})
	if candidateAllocs > baselineAllocs-0.5 {
		t.Fatalf("one-result allocations = %.1f, two-result baseline = %.1f; want at least one eliminated heap object", candidateAllocs, baselineAllocs)
	}
}

func BenchmarkDirectWriteShapingResultAllocations(b *testing.B) {
	request := directWriteAllocationRequest(b)
	b.Run("two-result-baseline", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			shaped, ok := directWriteTwoResultBaseline(request)
			if !ok {
				b.Fatal("baseline shape failed")
			}
			directWriteShapingAllocationSink = shaped
		}
	})
	b.Run("one-result-root-concrete", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			shaped, ok := shapeWithDirectWrite(request.Text, request.Face.Path, request.Face.Index, request.PPEM, request.Features)
			if !ok {
				b.Fatal("one-result shape failed")
			}
			directWriteShapingAllocationSink = shaped
		}
	})
}

func directWriteTwoResultBaseline(request platformpkg.ShapeRequest) ([]ShapedGlyph, bool) {
	platformGlyphs, ok := platformpkg.Shape(request)
	if !ok {
		return nil, false
	}
	shaped := make([]ShapedGlyph, len(platformGlyphs))
	for index, glyph := range platformGlyphs {
		assignRootShapedGlyph(&shaped[index], glyph)
	}
	return shaped, true
}

func directWriteAllocationRequest(tb testing.TB) platformpkg.ShapeRequest {
	tb.Helper()
	if !platformpkg.Available() {
		tb.Skip("DirectWrite unavailable")
	}
	tb.Setenv("LocalAppData", tb.TempDir())
	path, err := platformpkg.CachedGoMonoPath()
	if err != nil {
		tb.Fatalf("cache Go Mono: %v", err)
	}
	return platformpkg.ShapeRequest{Text: "office", Face: platformpkg.FaceSource{Path: path}, PPEM: 19}
}

var (
	_ func(string, loadedFace, uint16) ([]ShapedGlyph, bool)                      = (DirectWriteShaper{}).Shape
	_ func(string, loadedFace, uint16, fontdesc.FeatureSet) ([]ShapedGlyph, bool) = (DirectWriteShaper{}).ShapeFeatures
)
