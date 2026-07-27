package shape

import (
	"runtime"
	"strings"
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

func TestPolicyCachesUseHardBudget(t *testing.T) {
	policy, err := NewPolicy(PolicyConfig{Environment: testPolicyEnvironment(t)})
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

func TestPolicyPositiveCacheEvictsInDeterministicFIFOOrder(t *testing.T) {
	policy, err := NewPolicy(PolicyConfig{Environment: testPolicyEnvironment(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer policy.Close()
	keys := make([]ContentKey, fontdesc.MaxNegativeEntries+2)
	policy.mu.Lock()
	for index := 0; index < fontdesc.MaxNegativeEntries; index++ {
		keys[index] = ContentKey{Content: string(rune(index + 1))}
		policy.rememberLocked(keys[index], Plan{AuthoredIndex: uint32(index)})
	}
	policy.rememberLocked(keys[0], Plan{AuthoredIndex: 99})
	keys[fontdesc.MaxNegativeEntries] = ContentKey{Content: string(rune(fontdesc.MaxNegativeEntries + 1))}
	policy.rememberLocked(keys[fontdesc.MaxNegativeEntries], Plan{})
	_, oldestRetained := policy.resolved[keys[0]]
	_, secondRetained := policy.resolved[keys[1]]
	policy.mu.Unlock()
	if oldestRetained || !secondRetained {
		t.Fatalf("first eviction retained oldest=%v second=%v", oldestRetained, secondRetained)
	}
	policy.mu.Lock()
	keys[fontdesc.MaxNegativeEntries+1] = ContentKey{Content: string(rune(fontdesc.MaxNegativeEntries + 2))}
	policy.rememberLocked(keys[fontdesc.MaxNegativeEntries+1], Plan{})
	_, secondRetained = policy.resolved[keys[1]]
	_, thirdRetained := policy.resolved[keys[2]]
	entries := len(policy.resolved)
	policy.mu.Unlock()
	if secondRetained || !thirdRetained || entries != fontdesc.MaxNegativeEntries {
		t.Fatalf("second eviction retained second=%v third=%v entries=%d", secondRetained, thirdRetained, entries)
	}
}

func TestPolicyResolveCloseLifecycleRetainsInputsUntilInflightCompletes(t *testing.T) {
	descriptors := []fontdesc.Descriptor{{Family: "Primary"}}
	rules := []fontdesc.Rule{{
		Match: fontdesc.RuleMatch{Ranges: []fontdesc.RuneRange{{First: 'A', Last: 'A'}}},
		Use:   fontdesc.Descriptor{Family: "Rule"},
	}}
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: descriptors, Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	resolver := NewResolver(func(family string) []SourceFace {
		if family == "Rule" {
			close(entered)
			<-release
			return nil
		}
		if family == "Primary" {
			return []SourceFace{{Source: "test:primary", Metadata: testMetadata("Primary")}}
		}
		return nil
	})
	policy, err := NewPolicy(PolicyConfig{
		Resolver: resolver, Environment: environment, Descriptors: descriptors, Rules: rules,
		Hooks: PolicyHooks{
			Load:   func(Plan) bool { return true },
			Covers: func(plan Plan, _ string) bool { return plan.Selected.Source == "test:primary" },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved := make(chan struct {
		plan Plan
		ok   bool
	}, 1)
	go func() {
		plan, ok := policy.Resolve(fontdesc.RequestedFaceStyleNormal, "A")
		resolved <- struct {
			plan Plan
			ok   bool
		}{plan: plan, ok: ok}
	}()
	<-entered
	closed := make(chan struct{}, 2)
	go func() { policy.Close(); closed <- struct{}{} }()
	for !policy.closing.Load() {
		runtime.Gosched()
	}
	go func() { policy.Close(); closed <- struct{}{} }()
	for range 4 {
		runtime.Gosched()
	}
	select {
	case <-closed:
		t.Fatal("Close returned while resolution callback was in flight")
	default:
	}
	if _, ok := policy.Resolve(fontdesc.RequestedFaceStyleNormal, "B"); ok {
		t.Fatal("Resolve accepted new work after Close began")
	}
	close(release)
	result := <-resolved
	if !result.ok || result.plan.Tier != fontdesc.SourceTierPrimary {
		t.Fatalf("in-flight resolution = %#v, %v", result.plan, result.ok)
	}
	<-closed
	<-closed
	if stats := policy.Stats(); !stats.Closed || stats.InFlight != 0 || stats.ResolvedEntries != 0 || stats.LoadFailedEntries != 0 {
		t.Fatalf("closed policy stats = %+v", stats)
	}
	if policy.descriptors != nil || policy.rules != nil || policy.fallback != nil {
		t.Fatal("Close retained authored policy inputs")
	}
	if _, ok := policy.Resolve(fontdesc.RequestedFaceStyleNormal, "A"); ok {
		t.Fatal("Resolve returned a cached result after Close")
	}
}

func TestNewPolicyValidatesAllAuthoredBudgets(t *testing.T) {
	environment := testPolicyEnvironment(t)
	rangeSet := func(count, offset int) []fontdesc.RuneRange {
		ranges := make([]fontdesc.RuneRange, count)
		for index := range ranges {
			value := rune(offset + index*2)
			ranges[index] = fontdesc.RuneRange{First: value, Last: value}
		}
		return ranges
	}
	validRule := func(ranges []fontdesc.RuneRange) fontdesc.Rule {
		return fontdesc.Rule{Match: fontdesc.RuleMatch{Ranges: ranges}, Use: fontdesc.Descriptor{Family: "Rule"}}
	}
	tooManyRules := make([]fontdesc.Rule, fontdesc.MaxRules+1)
	totalRangeRules := make([]fontdesc.Rule, fontdesc.MaxTotalRanges/fontdesc.MaxRangesPerRule+1)
	for index := range totalRangeRules {
		totalRangeRules[index] = validRule(rangeSet(fontdesc.MaxRangesPerRule, 1+index*fontdesc.MaxRangesPerRule*2))
	}
	cases := []struct {
		name   string
		config PolicyConfig
	}{
		{name: "zero environment", config: PolicyConfig{}},
		{name: "primary count", config: PolicyConfig{Environment: environment, Descriptors: make([]fontdesc.Descriptor, fontdesc.MaxPrimaryDescriptors+1)}},
		{name: "fallback count", config: PolicyConfig{Environment: environment, Fallback: make([]fontdesc.Descriptor, fontdesc.MaxFallbackDescriptors+1)}},
		{name: "rule count", config: PolicyConfig{Environment: environment, Rules: tooManyRules}},
		{name: "ranges per rule", config: PolicyConfig{Environment: environment, Rules: []fontdesc.Rule{validRule(rangeSet(fontdesc.MaxRangesPerRule+1, 1))}}},
		{name: "total ranges", config: PolicyConfig{Environment: environment, Rules: totalRangeRules}},
		{name: "descriptor payload", config: PolicyConfig{Environment: environment, Descriptors: []fontdesc.Descriptor{{Family: strings.Repeat("x", fontdesc.MaxDescriptorPayloadBytes)}}}},
		{name: "feature payload", config: PolicyConfig{Environment: environment, FeaturePayload: make([]byte, fontdesc.MaxDescriptorPayloadBytes+1)}},
		{name: "feature count", config: PolicyConfig{Environment: environment, FeaturePayload: []byte{1, 0, fontdesc.MaxEffectiveFeatures + 1}}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if policy, err := NewPolicy(item.config); err == nil {
				policy.Close()
				t.Fatal("NewPolicy accepted invalid config")
			}
		})
	}
}

func testPolicyEnvironment(t *testing.T) fontdesc.FontEnvironmentKey {
	t.Helper()
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{})
	if err != nil {
		t.Fatal(err)
	}
	return environment
}

func testMetadata(family string) fontdesc.FaceMetadata {
	return fontdesc.FaceMetadata{Family: family, Subfamily: "Regular", Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100}.Normalized()
}
