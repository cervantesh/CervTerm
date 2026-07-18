package fontglyph

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
)

func TestFallbackBackendResolutionOrderAndLazyLoads(t *testing.T) {
	dir := t.TempDir()
	faces := []faceInfo{
		descriptorTestFace(t, dir, "primary.ttf", "Primary", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
		descriptorTestFace(t, dir, "rule.ttf", "Rule Face", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
		descriptorTestFace(t, dir, "fallback-one.ttf", "Fallback One", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
		descriptorTestFace(t, dir, "fallback-two.ttf", "Fallback Two", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
	}
	primary := []fontdesc.Descriptor{{Family: "Primary"}}
	fallback := []fontdesc.Descriptor{{Family: "Fallback One"}, {Family: "Fallback Two"}}
	rules := []fontdesc.Rule{{Match: fontdesc.RuleMatch{Ranges: []fontdesc.RuneRange{{First: '★', Last: '★'}}}, Use: fontdesc.Descriptor{Family: "Rule Face"}}}
	loads := 0
	coverageChecks := 0
	backend := newFallbackTestBackend(t, primary, fallback, rules, faces, func(candidate *OpenTypeBackend, content string) bool {
		coverageChecks++
		name := filepath.Base(candidate.faces[0].sourcePath)
		switch name {
		case "primary.ttf":
			return content == "A"
		case "rule.ttf":
			return content == "★"
		case "fallback-one.ttf":
			return content == "漢"
		case "fallback-two.ttf":
			return true
		default:
			return false
		}
	}, &loads)
	defer backend.Close()

	selected, ok := backend.resolveContent(fontdesc.RequestedFaceStyleNormal, "A")
	if !ok || selected.plan.tier != fontdesc.SourceTierPrimary || loads != 0 {
		t.Fatalf("ASCII selection = tier %d ok=%v loads=%d; want primary without lazy load", selected.plan.tier, ok, loads)
	}
	selected, ok = backend.resolveContent(fontdesc.RequestedFaceStyleNormal, "★")
	if !ok || selected.plan.tier != fontdesc.SourceTierRule || filepath.Base(selected.plan.selected.path) != "rule.ttf" || loads != 1 {
		t.Fatalf("rule selection = %#v ok=%v loads=%d", selected.plan, ok, loads)
	}
	selected, ok = backend.resolveContent(fontdesc.RequestedFaceStyleNormal, "漢")
	if !ok || selected.plan.tier != fontdesc.SourceTierFallback || filepath.Base(selected.plan.selected.path) != "fallback-one.ttf" || loads != 2 {
		t.Fatalf("fallback selection = %#v ok=%v loads=%d", selected.plan, ok, loads)
	}
	checksBeforeCachedLookup := coverageChecks
	if _, _, ok := backend.ClusterResolution(fontdesc.RequestedFaceStyleNormal, "漢"); !ok || loads != 2 {
		t.Fatalf("cached fallback resolution reloaded face: ok=%v loads=%d", ok, loads)
	}
	if coverageChecks != checksBeforeCachedLookup {
		t.Fatalf("cached resolution repeated coverage/shaping: %d -> %d", checksBeforeCachedLookup, coverageChecks)
	}
}

func TestFallbackBackendRuleRequiresCompleteClusterMatch(t *testing.T) {
	dir := t.TempDir()
	faces := []faceInfo{
		descriptorTestFace(t, dir, "primary.ttf", "Primary", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
		descriptorTestFace(t, dir, "rule.ttf", "Rule Face", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
		descriptorTestFace(t, dir, "fallback.ttf", "Fallback", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
	}
	rules := []fontdesc.Rule{{Match: fontdesc.RuleMatch{Class: fontdesc.SymbolClassEmoji}, Use: fontdesc.Descriptor{Family: "Rule Face"}}}
	loads := 0
	backend := newFallbackTestBackend(t,
		[]fontdesc.Descriptor{{Family: "Primary"}}, []fontdesc.Descriptor{{Family: "Fallback"}}, rules, faces,
		func(candidate *OpenTypeBackend, content string) bool {
			return filepath.Base(candidate.faces[0].sourcePath) != "primary.ttf"
		}, &loads,
	)
	defer backend.Close()
	selected, ok := backend.resolveContent(fontdesc.RequestedFaceStyleNormal, "😀A")
	if !ok || selected.plan.tier != fontdesc.SourceTierFallback || filepath.Base(selected.plan.selected.path) != "fallback.ttf" {
		t.Fatalf("mixed cluster selected %#v, ok=%v; want fallback after rule mismatch", selected.plan, ok)
	}
	if loads != 1 {
		t.Fatalf("mixed cluster loaded %d lazy faces, want only winning fallback", loads)
	}
}

func TestBackendCoversClusterChecksCombiningAndShaping(t *testing.T) {
	dir := t.TempDir()
	face := descriptorTestFace(t, dir, "mono.ttf", "Coverage", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF)
	primary, metrics, err := loadResolvedFacePlan(Spec{Size: 14, DPI: 96}, resolvedFacePlan{selected: faceCandidate{path: face.path, index: 0}})
	if err != nil {
		t.Fatal(err)
	}
	backend := newOpenTypeBackendFromPrimary(Spec{Size: 14, DPI: 96}, primary, metrics)
	backend.fallbacksLoaded = true
	defer backend.Close()
	if !backendCoversCluster(backend, "A") {
		t.Fatal("covered base scalar rejected")
	}
	if backendCoversCluster(backend, "A\u0301") {
		t.Fatal("cluster with uncovered combining mark accepted")
	}
	if backendCoversCluster(backend, "漢") {
		t.Fatal("missing CJK scalar accepted")
	}
}

func newFallbackTestBackend(t *testing.T, descriptors, fallback []fontdesc.Descriptor, rules []fontdesc.Rule, faces []faceInfo, covers func(*OpenTypeBackend, string) bool, loads *int) *fallbackBackend {
	t.Helper()
	manager := newFontCacheManager(fontdesc.MaxParsedFaces, fontdesc.MaxParsedBytes)
	restore := resetFontCacheForTest(manager)
	t.Cleanup(restore)
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: descriptors, Fallback: fallback, Rules: rules, DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	index := descriptorTestIndex(faces)
	primary, err := newDescriptorBackend(Spec{Size: 14, DPI: 96}, environment, descriptors, index)
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range primary.backends {
		child.fallbacksLoaded = true
	}
	backend := &fallbackBackend{
		primary: primary, spec: Spec{Size: 14, DPI: 96}, environment: environment, index: index,
		descriptors: descriptors, fallback: fallback, rules: cloneResolvedRules(rules),
		loaded: make(map[fontdesc.ResolvedFaceKey]*OpenTypeBackend), loadFailed: make(map[fontdesc.ResolvedFaceKey]struct{}), resolved: make(map[contentResolutionKey]fallbackSelection), covers: covers,
	}
	backend.load = func(spec Spec, plan resolvedFacePlan) (loadedFace, font.Metrics, error) {
		(*loads)++
		return loadResolvedFacePlan(spec, plan)
	}
	return backend
}

func TestFallbackResolvedIdentityChangesByTierAndSource(t *testing.T) {
	// Guard against accidentally collapsing rule/fallback identities onto the
	// primary style key in atlas caches.
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: []fontdesc.Descriptor{{Family: "P"}}})
	if err != nil {
		t.Fatal(err)
	}
	face := fontdesc.CanonicalFaceIDFromBytes([]byte("same-face"))
	base := fontdesc.ResolvedFaceInput{Environment: environment, Face: face, Target: fontdesc.FaceTarget{Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100}}
	base.Tier = fontdesc.SourceTierPrimary
	primary, _ := fontdesc.NewResolvedFaceKey(base)
	base.Tier = fontdesc.SourceTierFallback
	fallback, _ := fontdesc.NewResolvedFaceKey(base)
	if primary == fallback || strings.TrimSpace(primary.String()) == "" || strings.TrimSpace(fallback.String()) == "" {
		t.Fatal("resolved identity collapsed across source tiers")
	}
}

func TestFallbackBackendUsesEmbeddedAsStableFinalResolution(t *testing.T) {
	dir := t.TempDir()
	face := descriptorTestFace(t, dir, "primary.ttf", "Primary", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF)
	loads := 0
	backend := newFallbackTestBackend(t, []fontdesc.Descriptor{{Family: "Primary"}}, nil, nil, []faceInfo{face}, func(*OpenTypeBackend, string) bool { return false }, &loads)
	defer backend.Close()
	selected, ok := backend.resolveContent(fontdesc.RequestedFaceStyleNormal, "漢")
	if !ok || selected.plan.tier != fontdesc.SourceTierEmbedded || selected.plan.resolvedKey == (fontdesc.ResolvedFaceKey{}) || loads != 1 {
		t.Fatalf("embedded final selection = %#v ok=%v loads=%d", selected.plan, ok, loads)
	}
	if _, ok := backend.RasterizeStyle(fontdesc.RequestedFaceStyleNormal, '漢', 2); ok {
		t.Fatal("embedded Go Mono unexpectedly rasterized missing CJK glyph")
	}
	if loads != 1 {
		t.Fatalf("embedded negative resolution reloaded face: %d", loads)
	}
}

func TestFallbackBackendContinuesRulesAfterLoadFailure(t *testing.T) {
	dir := t.TempDir()
	faces := []faceInfo{
		descriptorTestFace(t, dir, "primary.ttf", "Primary", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
		descriptorTestFace(t, dir, "bad.ttf", "Bad Rule", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
		descriptorTestFace(t, dir, "good.ttf", "Good Rule", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
	}
	rules := []fontdesc.Rule{
		{Match: fontdesc.RuleMatch{Class: fontdesc.SymbolClassSymbols}, Use: fontdesc.Descriptor{Family: "Bad Rule"}},
		{Match: fontdesc.RuleMatch{Class: fontdesc.SymbolClassSymbols}, Use: fontdesc.Descriptor{Family: "Good Rule"}},
	}
	loads := 0
	backend := newFallbackTestBackend(t, []fontdesc.Descriptor{{Family: "Primary"}}, nil, rules, faces, func(candidate *OpenTypeBackend, _ string) bool {
		return filepath.Base(candidate.faces[0].sourcePath) == "good.ttf"
	}, &loads)
	defer backend.Close()
	baseLoad := backend.load
	attempts := 0
	backend.load = func(spec Spec, plan resolvedFacePlan) (loadedFace, font.Metrics, error) {
		attempts++
		if filepath.Base(plan.selected.path) == "bad.ttf" {
			return loadedFace{}, font.Metrics{}, errors.New("injected load failure")
		}
		return baseLoad(spec, plan)
	}
	selected, ok := backend.resolveContent(fontdesc.RequestedFaceStyleNormal, "→")
	if !ok || selected.plan.tier != fontdesc.SourceTierRule || selected.plan.authoredIndex != 1 || filepath.Base(selected.plan.selected.path) != "good.ttf" || attempts != 2 {
		t.Fatalf("rule failover = %#v ok=%v attempts=%d", selected.plan, ok, attempts)
	}
}

func TestFallbackBackendReleasesLosingCandidatePins(t *testing.T) {
	dir := t.TempDir()
	faces := []faceInfo{
		descriptorTestFace(t, dir, "primary.ttf", "Primary", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
		descriptorTestFace(t, dir, "loser.ttf", "Loser", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
		descriptorTestFace(t, dir, "winner.ttf", "Winner", "Regular", 400, fontdesc.StyleNormal, 0, gomono.TTF),
	}
	loads := 0
	backend := newFallbackTestBackend(t, []fontdesc.Descriptor{{Family: "Primary"}}, []fontdesc.Descriptor{{Family: "Loser"}, {Family: "Winner"}}, nil, faces, func(candidate *OpenTypeBackend, _ string) bool {
		return filepath.Base(candidate.faces[0].sourcePath) == "winner.ttf"
	}, &loads)
	manager := currentFontCache()
	if before := manager.stats().Pinned; before != 4 {
		t.Fatalf("primary pins = %d, want 4", before)
	}
	selected, ok := backend.resolveContent(fontdesc.RequestedFaceStyleNormal, "漢")
	if !ok || filepath.Base(selected.plan.selected.path) != "winner.ttf" || loads != 2 {
		t.Fatalf("fallback failover = %#v ok=%v loads=%d", selected.plan, ok, loads)
	}
	if after := manager.stats().Pinned; after != 5 {
		t.Fatalf("pins after losing/winning candidates = %d, want 5", after)
	}
	backend.Close()
	if final := manager.stats().Pinned; final != 0 {
		t.Fatalf("pins after close = %d, want 0", final)
	}
}

func TestFallbackResolutionCacheIsBounded(t *testing.T) {
	backend := &fallbackBackend{resolved: make(map[contentResolutionKey]fallbackSelection), loadFailed: make(map[fontdesc.ResolvedFaceKey]struct{})}
	for index := 0; index <= fontdesc.MaxNegativeEntries; index++ {
		key := contentResolutionKey{content: string(rune(index + 1))}
		backend.rememberResolution(key, fallbackSelection{})
	}
	if len(backend.resolved) != fontdesc.MaxNegativeEntries || len(backend.resolvedRing) != fontdesc.MaxNegativeEntries {
		t.Fatalf("resolution cache/ring = %d/%d, want %d", len(backend.resolved), len(backend.resolvedRing), fontdesc.MaxNegativeEntries)
	}
	if _, retained := backend.resolved[contentResolutionKey{content: string(rune(1))}]; retained {
		t.Fatal("oldest resolution was not evicted")
	}
	for index := 0; index <= fontdesc.MaxNegativeEntries; index++ {
		var key fontdesc.ResolvedFaceKey
		key[0], key[1] = byte(index), byte(index>>8)
		backend.recordLoadFailure(key)
	}
	if len(backend.loadFailed) != fontdesc.MaxNegativeEntries || len(backend.loadFailedRing) != fontdesc.MaxNegativeEntries {
		t.Fatalf("load-failure cache/ring = %d/%d, want %d", len(backend.loadFailed), len(backend.loadFailedRing), fontdesc.MaxNegativeEntries)
	}
}
