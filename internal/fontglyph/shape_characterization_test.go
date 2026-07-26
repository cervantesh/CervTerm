package fontglyph

import (
	"reflect"
	"testing"

	"cervterm/internal/fontdesc"
	shapepkg "cervterm/internal/fontglyph/shape"

	"golang.org/x/image/font/gofont/gomono"
)

// Compile-time pins for the legacy root facade. Slice 5.5b must keep these
// concrete root identities and signatures while moving their implementation.
var (
	_ Shaper                             = SimpleShaper{}
	_ FeatureShaper                      = SimpleShaper{}
	_ func(Backend, fontdesc.FeatureSet) = ConfigureBackendFeatures
	_ func(Backend) string               = BackendFeatureCapability
	_ func(*OpenTypeBackend, Shaper)     = (*OpenTypeBackend).SetShaper
)

func TestL402ShapeRootConcreteCompatibility(t *testing.T) {
	glyph := reflect.TypeOf(ShapedGlyph{})
	if glyph.PkgPath() != "cervterm/internal/fontglyph" || glyph.Name() != "ShapedGlyph" || glyph.NumField() != 4 || glyph.NumMethod() != 0 {
		t.Fatalf("ShapedGlyph identity/method set = %s.%s fields=%d methods=%d", glyph.PkgPath(), glyph.Name(), glyph.NumField(), glyph.NumMethod())
	}
	wantFields := []string{"GlyphID", "XOffset", "YOffset", "XAdvance"}
	for index, want := range wantFields {
		if got := glyph.Field(index).Name; got != want {
			t.Fatalf("ShapedGlyph field %d = %q, want %q", index, got, want)
		}
	}
	simple := reflect.TypeOf(SimpleShaper{})
	if simple.PkgPath() != "cervterm/internal/fontglyph" || simple.Name() != "SimpleShaper" || simple.Kind() != reflect.Struct || simple.NumField() != 0 || simple.NumMethod() != 3 {
		t.Fatalf("SimpleShaper identity/method set = %s.%s kind=%s fields=%d methods=%d", simple.PkgPath(), simple.Name(), simple.Kind(), simple.NumField(), simple.NumMethod())
	}
}

func TestL402RootPortableShapingAdaptersPreserveLeafOutput(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	source := backend.faces[0]
	adapted := rootShaperFromShape(shapepkg.Simple{})
	if _, unwrapped := adapted.(SimpleShaper); unwrapped {
		t.Fatal("rootShaperFromShape unwrapped shape.Simple to the root concrete type")
	}
	if !isPortableShaper(adapted) {
		t.Fatal("shape.Simple adapter lost portable-shaper behavior")
	}
	for _, cluster := range []string{"A", "e\u0301", "->", "م"} {
		got, gotOK := (SimpleShaper{}).Shape(cluster, source, backend.ppem)
		leaf, leafOK := (shapepkg.Simple{}).Shape(cluster, shapingFaceRef(source), backend.ppem)
		want := shapedGlyphsFromShape(leaf)
		if gotOK != leafOK || !reflect.DeepEqual(got, want) {
			t.Fatalf("cluster %q root=%#v/%v leaf=%#v/%v", cluster, got, gotOK, want, leafOK)
		}
		adaptedOutput, adaptedOK := adapted.Shape(cluster, source, backend.ppem)
		if adaptedOK != leafOK || !reflect.DeepEqual(adaptedOutput, want) {
			t.Fatalf("cluster %q adapter=%#v/%v leaf=%#v/%v", cluster, adaptedOutput, adaptedOK, want, leafOK)
		}
	}
	shaped, ok := adapted.Shape("->", source, backend.ppem)
	if !ok {
		t.Fatal("portable pair shaping failed")
	}
	if got, want := runSubstituted(adapted, source, backend.ppem, "->", shaped, fontdesc.FeatureSet{}), shapepkg.RunSubstituted(shapepkg.Simple{}, shapingFaceRef(source), backend.ppem, "->", shapedGlyphsToShape(shaped), fontdesc.FeatureSet{}); got != want {
		t.Fatalf("run substitution root=%v leaf=%v", got, want)
	}
	if got, want := centerShapedGlyphsInCells(shaped, backend.cellW*2), shapedGlyphsFromShape(shapepkg.CenterInCells(shapedGlyphsToShape(shaped), backend.cellW*2)); !reflect.DeepEqual(got, want) {
		t.Fatalf("centering root=%#v leaf=%#v", got, want)
	}
}

func TestL402SimpleShapeExactOutputAndDetachedCentering(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	face := backend.faces[0]
	got, ok := (SimpleShaper{}).Shape("A", face, backend.ppem)
	if !ok || len(got) != 1 {
		t.Fatalf("shape A = %#v, %v", got, ok)
	}
	var wantID uint16
	one, oneOK := shapeOneRune(face.sfnt, 'A', backend.ppem)
	if oneOK && len(one) == 1 {
		wantID = one[0].GlyphID
	}
	if !oneOK || got[0].GlyphID != wantID || got[0].XOffset != 0 || got[0].YOffset != 0 || got[0].XAdvance != one[0].XAdvance || got[0].XAdvance <= 0 {
		t.Fatalf("shape A exact output = %#v, scalar baseline %#v", got, one)
	}
	centered := centerShapedGlyphsInCells(got, backend.cellW)
	if got[0].XOffset != 0 || len(centered) != 1 || centered[0].GlyphID != got[0].GlyphID || centered[0].XAdvance != got[0].XAdvance {
		t.Fatalf("centering mutated or changed glyph output: input=%#v centered=%#v", got, centered)
	}
}

func TestL402ResolverSourceOrderIdentityAndFallbackBudgets(t *testing.T) {
	descriptors := []fontdesc.Descriptor{{Family: "Missing"}, {Family: "Earlier"}, {Family: "Later"}}
	faces := []faceInfo{
		resolverNamedTestFace("Later", "test:z", 0, "Regular", 400, fontdesc.StyleNormal, 100),
		resolverNamedTestFace("Earlier", "test:a", 0, "Regular", 400, fontdesc.StyleNormal, 100),
	}
	plans, err := resolvePrimaryFacePlan(resolverTestIndex(faces), resolverTestEnvironment(t, descriptors), descriptors, fontdesc.RequestedFaceStyleNormal)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].selected.path != "test:a" || plans[0].authoredIndex != 1 || plans[1].selected.path != "test:z" || plans[1].authoredIndex != 2 {
		t.Fatalf("authored source order = %#v", plans)
	}
	backend := &fallbackBackend{resolved: make(map[contentResolutionKey]fallbackSelection), loadFailed: make(map[fontdesc.ResolvedFaceKey]struct{})}
	for index := 0; index <= fontdesc.MaxNegativeEntries; index++ {
		backend.rememberResolution(contentResolutionKey{content: string(rune(index + 1))}, fallbackSelection{})
		var key fontdesc.ResolvedFaceKey
		key[0], key[1] = byte(index), byte(index>>8)
		backend.recordLoadFailure(key)
	}
	if len(backend.resolved) != fontdesc.MaxNegativeEntries || len(backend.loadFailed) != fontdesc.MaxNegativeEntries {
		t.Fatalf("bounded caches = resolved %d failed %d", len(backend.resolved), len(backend.loadFailed))
	}
}

func BenchmarkL402ResolvePrimaryPlans(b *testing.B) {
	descriptor := fontdesc.Descriptor{Family: "Go Mono"}
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: []fontdesc.Descriptor{descriptor}, DPI: 96})
	if err != nil {
		b.Fatal(err)
	}
	metadata := fontdesc.FaceMetadata{Family: "Go Mono", Subfamily: "Regular", Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100}.Normalized()
	index := newFontIndex([]faceInfo{newFaceInfo("embedded:gomono", 0, metadata.Family, metadata.Subfamily, metadata)})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		plans, resolveErr := resolvePrimaryFacePlan(index, environment, []fontdesc.Descriptor{descriptor}, fontdesc.RequestedFaceStyleNormal)
		if resolveErr != nil || len(plans) != 1 {
			b.Fatalf("plans=%d err=%v", len(plans), resolveErr)
		}
	}
}

func BenchmarkL402SimpleShapeASCII(b *testing.B) {
	face, _, err := loadOpenTypeFace(gomono.TTF, Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		b.Fatal(err)
	}
	defer closeLoadedFace(&face)
	shaper := SimpleShaper{}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		glyphs, ok := shaper.Shape("A", face, 19)
		if !ok || len(glyphs) != 1 {
			b.Fatal("shape failed")
		}
	}
}

func BenchmarkL402FallbackResolutionHit(b *testing.B) {
	descriptor := fontdesc.Descriptor{Family: "Primary"}
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: []fontdesc.Descriptor{descriptor}})
	if err != nil {
		b.Fatal(err)
	}
	resolver := shapepkg.NewResolver(func(string) []shapepkg.SourceFace {
		return []shapepkg.SourceFace{{Source: "test:primary", Metadata: fontdesc.FaceMetadata{Family: "Primary", Subfamily: "Regular", Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100}}}
	})
	plans, err := resolver.PrimaryPlans(environment, []fontdesc.Descriptor{descriptor}, fontdesc.RequestedFaceStyleNormal)
	if err != nil {
		b.Fatal(err)
	}
	policy, err := shapepkg.NewPolicy(shapepkg.PolicyConfig{
		Resolver: resolver, Environment: environment, Descriptors: []fontdesc.Descriptor{descriptor},
		Hooks: shapepkg.PolicyHooks{
			Primary:       func(fontdesc.RequestedFaceStyle) (shapepkg.Plan, bool) { return plans[0], true },
			PrimaryCovers: func(fontdesc.RequestedFaceStyle, string) bool { return true },
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer policy.Close()
	if _, ok := policy.Resolve(fontdesc.RequestedFaceStyleNormal, "A"); !ok {
		b.Fatal("seed resolution failed")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		plan, ok := policy.Resolve(fontdesc.RequestedFaceStyleNormal, "A")
		if !ok || plan.Tier != fontdesc.SourceTierPrimary {
			b.Fatal("cached resolution failed")
		}
	}
}
