package shape

import (
	"sync"
	"sync/atomic"
	"testing"

	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph/internal/face"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/sfnt"
)

func TestResolverPreservesAuthoredAndCanonicalOrder(t *testing.T) {
	descriptors := []fontdesc.Descriptor{{Family: "Missing"}, {Family: "Earlier"}, {Family: "Later"}}
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: descriptors})
	if err != nil {
		t.Fatal(err)
	}
	faces := map[string][]SourceFace{
		"Earlier": {{Source: "test:a", Metadata: testMetadata("Earlier")}},
		"Later":   {{Source: "test:z", Metadata: testMetadata("Later")}},
	}
	resolver := NewResolver(func(family string) []SourceFace { return append([]SourceFace(nil), faces[family]...) })
	plans, err := resolver.PrimaryPlans(environment, descriptors, fontdesc.RequestedFaceStyleNormal)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].Selected.Source != "test:a" || plans[0].AuthoredIndex != 1 || plans[1].Selected.Source != "test:z" || plans[1].AuthoredIndex != 2 {
		t.Fatalf("ordered plans = %#v", plans)
	}
	if plans[0].CanonicalFaceID == (fontdesc.CanonicalFaceID{}) || plans[0].ResolvedKey == (fontdesc.ResolvedFaceKey{}) {
		t.Fatal("stable identities were not constructed")
	}
}

func TestSimpleAndRunShapingExactOutputs(t *testing.T) {
	parsed, err := sfnt.Parse(gomono.TTF)
	if err != nil {
		t.Fatal(err)
	}
	ref := face.NewRef(parsed, "embedded:gomono", 0)
	shaper := Simple{}
	glyphs, ok := shaper.Shape("A", ref, 19)
	if !ok || len(glyphs) != 1 || glyphs[0].GlyphID == 0 || glyphs[0].XOffset != 0 || glyphs[0].YOffset != 0 || glyphs[0].XAdvance <= 0 {
		t.Fatalf("simple output = %#v, %v", glyphs, ok)
	}
	pair, ok := shaper.Shape("->", ref, 19)
	if !ok || RunSubstituted(shaper, ref, 19, "->", pair, fontdesc.FeatureSet{}) {
		t.Fatalf("portable pair misclassified: %#v, %v", pair, ok)
	}
	centered := CenterInCells(glyphs, 20)
	if glyphs[0].XOffset != 0 || centered[0].GlyphID != glyphs[0].GlyphID || centered[0].XAdvance != glyphs[0].XAdvance {
		t.Fatalf("detached centering = input %#v output %#v", glyphs, centered)
	}
}

func TestPolicyLazyWholeClusterOrderCacheAndConcurrency(t *testing.T) {
	descriptors := []fontdesc.Descriptor{{Family: "Primary"}}
	fallback := []fontdesc.Descriptor{{Family: "Fallback"}}
	rules := []fontdesc.Rule{{Match: fontdesc.RuleMatch{Ranges: []fontdesc.RuneRange{{First: '★', Last: '★'}}}, Use: fontdesc.Descriptor{Family: "Rule"}}}
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: descriptors, Fallback: fallback, Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	faces := map[string][]SourceFace{
		"Primary":  {{Source: "test:primary", Metadata: testMetadata("Primary")}},
		"Rule":     {{Source: "test:rule", Metadata: testMetadata("Rule")}},
		"Fallback": {{Source: "test:fallback", Metadata: testMetadata("Fallback")}},
	}
	resolver := NewResolver(func(family string) []SourceFace { return append([]SourceFace(nil), faces[family]...) })
	primaryPlans, err := resolver.PrimaryPlans(environment, descriptors, fontdesc.RequestedFaceStyleNormal)
	if err != nil {
		t.Fatal(err)
	}
	primary := primaryPlans[0]
	var loads atomic.Int32
	var callbackMu sync.Mutex
	loaded := make(map[fontdesc.ResolvedFaceKey]bool)
	policy, err := NewPolicy(PolicyConfig{
		Resolver: resolver, Environment: environment, Descriptors: descriptors, Fallback: fallback, Rules: rules,
		Hooks: PolicyHooks{
			Primary:       func(fontdesc.RequestedFaceStyle) (Plan, bool) { return primary, true },
			PrimaryCovers: func(_ fontdesc.RequestedFaceStyle, content string) bool { return content == "A" },
			Loaded:        func(plan Plan) bool { callbackMu.Lock(); defer callbackMu.Unlock(); return loaded[plan.ResolvedKey] },
			Load: func(plan Plan) bool {
				callbackMu.Lock()
				defer callbackMu.Unlock()
				if !loaded[plan.ResolvedKey] {
					loads.Add(1)
					loaded[plan.ResolvedKey] = true
				}
				return true
			},
			Covers: func(plan Plan, content string) bool {
				switch plan.Selected.Source {
				case "test:rule":
					return content == "★"
				case "test:fallback":
					return content == "漢字"
				default:
					return false
				}
			},
			Discard: func(plan Plan) { callbackMu.Lock(); delete(loaded, plan.ResolvedKey); callbackMu.Unlock() },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer policy.Close()
	if plan, ok := policy.Resolve(fontdesc.RequestedFaceStyleNormal, "A"); !ok || plan.Tier != fontdesc.SourceTierPrimary || loads.Load() != 0 {
		t.Fatalf("primary = %#v %v loads=%d", plan, ok, loads.Load())
	}
	if plan, ok := policy.Resolve(fontdesc.RequestedFaceStyleNormal, "★"); !ok || plan.Tier != fontdesc.SourceTierRule || loads.Load() != 1 {
		t.Fatalf("rule = %#v %v loads=%d", plan, ok, loads.Load())
	}
	const callers = 32
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			<-start
			plan, ok := policy.Resolve(fontdesc.RequestedFaceStyleNormal, "漢字")
			if !ok || plan.Tier != fontdesc.SourceTierFallback {
				t.Errorf("fallback = %#v, %v", plan, ok)
			}
		}()
	}
	close(start)
	wait.Wait()
	if loads.Load() != 2 {
		t.Fatalf("whole-cluster concurrent fallback loads = %d, want 2 total", loads.Load())
	}
	if stats := policy.Stats(); stats.ResolvedEntries != 3 || stats.InFlight != 0 {
		t.Fatalf("policy stats = %+v", stats)
	}
}

func TestPolicyNegativeBudgetIsBounded(t *testing.T) {
	policy, err := NewPolicy(PolicyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer policy.Close()
	for index := 0; index <= fontdesc.MaxNegativeEntries; index++ {
		var key fontdesc.ResolvedFaceKey
		key[0], key[1] = byte(index), byte(index>>8)
		policy.recordFailure(key)
	}
	if stats := policy.Stats(); stats.LoadFailedEntries != fontdesc.MaxNegativeEntries {
		t.Fatalf("negative entries = %d, want %d", stats.LoadFailedEntries, fontdesc.MaxNegativeEntries)
	}
}

func testMetadata(family string) fontdesc.FaceMetadata {
	return fontdesc.FaceMetadata{Family: family, Subfamily: "Regular", Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100}.Normalized()
}
