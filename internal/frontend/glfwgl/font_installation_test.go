//go:build glfw

package glfwgl

import (
	"errors"
	"strings"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph"
)

func testFontInstallationPlan(t *testing.T, backend *atlasTestBackend) fontInstallationPlan {
	t.Helper()
	cfg := config.Defaults()
	plan, err := newFontInstallationPlan(cfg, 96, "go", func(fontglyph.Spec) (fontglyph.Backend, error) {
		return backend, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestEffectiveStartupConfigSafeFontsIsIsolated(t *testing.T) {
	authored := config.Defaults()
	authored.Font.Family = "Configured Family"
	authored.Font.Descriptors = []fontdesc.Descriptor{{Family: "Configured Family"}}
	normal := effectiveStartupConfig(authored, false)
	safe := effectiveStartupConfig(authored, true)
	if normal.Font.Family != "Configured Family" || len(normal.Font.Descriptors) != 1 || safe.Font.Family != "Go Mono" || len(safe.Font.Descriptors) != 0 {
		t.Fatalf("normal/safe font configs = %#v/%#v", normal.Font, safe.Font)
	}
	if authored.Font.Family != "Configured Family" || len(authored.Font.Descriptors) != 1 {
		t.Fatal("safe mode mutated authored config")
	}
}

func TestStartupFontInstallationPlanRoutesDescriptorsAndSafeMode(t *testing.T) {
	cfg := config.Defaults()
	cfg.Font.Descriptors = []fontdesc.Descriptor{{Family: "Go Mono"}}
	descriptorPlan, err := newStartupFontInstallationPlan(cfg, 96, "go", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(descriptorPlan.descriptors) != 1 {
		t.Fatal("descriptor startup selected legacy plan")
	}
	safePlan, err := newStartupFontInstallationPlan(effectiveStartupConfig(cfg, true), 96, "go", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(safePlan.descriptors) != 0 || safePlan.spec.Family != "Go Mono" {
		t.Fatalf("safe plan = %#v", safePlan)
	}
}

func TestFontInstallationFailuresReleaseOwnedBackend(t *testing.T) {
	injected := errors.New("injected")
	for _, stage := range []string{"plan", "reserve", "load", "metrics", "context"} {
		t.Run(stage, func(t *testing.T) {
			backend := &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}
			plan := testFontInstallationPlan(t, backend)
			stages := defaultFontInstallationStages()
			switch stage {
			case "plan":
				stages.plan = func(fontInstallationPlan) error { return injected }
			case "reserve":
				stages.reserve = func(fontInstallationPlan) error { return injected }
			case "load":
				stages.load = func(fontInstallationPlan) (fontglyph.Backend, error) { return nil, injected }
			case "metrics":
				stages.metrics = func(fontglyph.Backend) (fontInstallationMetrics, error) { return fontInstallationMetrics{}, injected }
			case "context":
				stages.context = func(fontInstallationPlan, fontglyph.Backend, fontInstallationMetrics) (*atlasFontContext, error) {
					return nil, injected
				}
			}
			prepared, err := prepareFontInstallation(plan, stages)
			if !errors.Is(err, injected) || prepared != nil {
				t.Fatalf("prepared=%v error=%v", prepared, err)
			}
			wantClose := 0
			if stage == "metrics" || stage == "context" {
				wantClose = 1
			}
			if backend.closeCalls != wantClose {
				t.Fatalf("backend closes=%d, want %d", backend.closeCalls, wantClose)
			}
		})
	}
}

func TestFontInstallationAdoptionTransfersOwnershipOnce(t *testing.T) {
	backend := &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}
	plan := testFontInstallationPlan(t, backend)
	stages := defaultFontInstallationStages()
	prepared, err := prepareFontInstallation(plan, stages)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.context.key.environment == (fontdesc.FontEnvironmentKey{}) || prepared.context.resolvedFace == (fontdesc.ResolvedFaceKey{}) {
		t.Fatal("prepared context has zero identity")
	}
	renderer := &atlasTestRenderer{}
	atlas, err := prepared.adopt(renderer, stages)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Close()
	if backend.closeCalls != 0 {
		t.Fatalf("adopted backend closed early: %d", backend.closeCalls)
	}
	if _, err := prepared.adopt(renderer, stages); err == nil {
		t.Fatal("second adoption succeeded")
	}
	atlas.close()
	if backend.closeCalls != 1 || renderer.destroyCalls != 1 {
		t.Fatalf("close calls backend/renderer=%d/%d", backend.closeCalls, renderer.destroyCalls)
	}
}

func TestFontInstallationAdoptFailureRemainsCandidateOwned(t *testing.T) {
	backend := &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}
	plan := testFontInstallationPlan(t, backend)
	stages := defaultFontInstallationStages()
	prepared, err := prepareFontInstallation(plan, stages)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("adopt")
	stages.adopt = func(*preparedFontInstallation) error { return injected }
	if atlas, err := prepared.adopt(&atlasTestRenderer{}, stages); atlas != nil || !errors.Is(err, injected) {
		t.Fatalf("atlas=%v error=%v", atlas, err)
	}
	prepared.Close()
	prepared.Close()
	if backend.closeCalls != 1 {
		t.Fatalf("backend closes=%d, want 1", backend.closeCalls)
	}
}

func TestDescriptorFontInstallationRetainsEnvironmentAcrossAdoptionAndZoom(t *testing.T) {
	cfg := config.Defaults()
	descriptors := []fontdesc.Descriptor{{Family: "First Mono"}, {Family: "Second Mono", Weight: 700}}
	type construction struct {
		spec        fontglyph.Spec
		environment fontdesc.FontEnvironmentKey
		descriptors []fontdesc.Descriptor
		backend     *atlasStyledTestBackend
	}
	var constructions []construction
	construct := func(spec fontglyph.Spec, environment fontdesc.FontEnvironmentKey, gotDescriptors []fontdesc.Descriptor) (fontglyph.Backend, error) {
		backend := &atlasStyledTestBackend{
			atlasTestBackend: &atlasTestBackend{cellW: int(spec.Size), cellH: int(spec.Size) + 4, baseline: int(spec.Size)},
			environment:      environment,
			faceSalt:         environment.String(),
		}
		constructions = append(constructions, construction{spec: spec, environment: environment, descriptors: append([]fontdesc.Descriptor(nil), gotDescriptors...), backend: backend})
		return backend, nil
	}
	plan, err := newDescriptorFontInstallationPlanWithFactory(cfg, 96, "gray", descriptors, construct)
	if err != nil {
		t.Fatal(err)
	}
	descriptors[0].Family = "mutated by caller"
	if got := plan.descriptors[0].Family; got != "First Mono" {
		t.Fatalf("plan descriptor mutated through caller slice: %q", got)
	}

	stages := defaultFontInstallationStages()
	prepared, err := prepareFontInstallation(plan, stages)
	if err != nil {
		t.Fatal(err)
	}
	wantInitial, err := makeAtlasFontKeyWithDescriptors(plan.spec, plan.textGamma, plan.textDarken, plan.descriptors)
	if err != nil {
		t.Fatal(err)
	}
	if len(constructions) != 1 || len(constructions[0].descriptors) != 2 || constructions[0].environment != wantInitial.environment {
		t.Fatalf("initial descriptor construction = %#v, want complete ordered environment %s", constructions, wantInitial.environment)
	}
	if _, ok := activeStyledBackend(prepared.context); !ok || prepared.context.key.environment != wantInitial.environment {
		t.Fatal("prepared descriptor context is not styled or has the wrong environment")
	}

	renderer := &atlasTestRenderer{}
	atlas, err := prepared.adopt(renderer, stages)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := activeStyledBackend(atlas.activeContext); !ok {
		t.Fatal("adopted descriptor context lost styled backend behavior")
	}
	zoomed := plan.spec
	zoomed.Size += 3
	if _, _, _, ok := atlas.useSpec(zoomed, plan.textGamma, plan.textDarken); !ok {
		t.Fatal("descriptor atlas failed to create zoomed context")
	}
	wantZoomed, err := makeAtlasFontKeyWithDescriptors(zoomed, plan.textGamma, plan.textDarken, plan.descriptors)
	if err != nil {
		t.Fatal(err)
	}
	if len(constructions) != 2 || constructions[1].environment != wantZoomed.environment || constructions[1].environment == constructions[0].environment {
		t.Fatalf("zoom descriptor constructions = %#v", constructions)
	}
	if _, ok := activeStyledBackend(atlas.activeContext); !ok || atlas.activeContext.key.environment != wantZoomed.environment {
		t.Fatal("zoomed descriptor context lost styled behavior or reused startup environment")
	}
	retainedInitial, keyErr := atlas.fontKey(plan.spec, plan.textGamma, plan.textDarken)
	if keyErr != nil || retainedInitial != wantInitial {
		t.Fatalf("descriptor retention key = %#v, %v; want %#v", retainedInitial, keyErr, wantInitial)
	}
	retainedZoomed, keyErr := atlas.fontKey(zoomed, plan.textGamma, plan.textDarken)
	if keyErr != nil || retainedZoomed != wantZoomed {
		t.Fatalf("zoomed descriptor retention key = %#v, %v; want %#v", retainedZoomed, keyErr, wantZoomed)
	}
	atlas.retainContexts(map[atlasFontKey]struct{}{retainedInitial: {}, retainedZoomed: {}})
	if len(atlas.contexts) != 2 {
		t.Fatalf("descriptor contexts after retention = %d, want 2", len(atlas.contexts))
	}
	atlas.close()
	for i, constructed := range constructions {
		if constructed.backend.closeCalls != 1 {
			t.Fatalf("descriptor backend %d close calls = %d, want 1", i, constructed.backend.closeCalls)
		}
	}
}

func TestDescriptorFontInstallationPlanValidatesIdentityBounds(t *testing.T) {
	cfg := config.Defaults()
	construct := func(fontglyph.Spec, fontdesc.FontEnvironmentKey, []fontdesc.Descriptor) (fontglyph.Backend, error) {
		return &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}, nil
	}
	if _, err := newDescriptorFontInstallationPlanWithFactory(cfg, 96, "gray", nil, construct); err == nil || !strings.Contains(err.Error(), "no primary descriptors") {
		t.Fatalf("empty descriptor plan error = %v", err)
	}
	tooMany := make([]fontdesc.Descriptor, fontdesc.MaxPrimaryDescriptors+1)
	for i := range tooMany {
		tooMany[i] = fontdesc.Descriptor{Family: "Bounded Mono"}
	}
	if _, err := newDescriptorFontInstallationPlanWithFactory(cfg, 96, "gray", tooMany, construct); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("descriptor count plan error = %v", err)
	}
	oversized := []fontdesc.Descriptor{{Family: strings.Repeat("x", fontdesc.MaxDescriptorPayloadBytes)}}
	if _, err := newDescriptorFontInstallationPlanWithFactory(cfg, 96, "gray", oversized, construct); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("descriptor payload plan error = %v", err)
	}
}

func TestDescriptorFontInstallationAbortClosesCandidateBackend(t *testing.T) {
	cfg := config.Defaults()
	var backend *atlasStyledTestBackend
	construct := func(spec fontglyph.Spec, environment fontdesc.FontEnvironmentKey, _ []fontdesc.Descriptor) (fontglyph.Backend, error) {
		backend = &atlasStyledTestBackend{atlasTestBackend: &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}, environment: environment, faceSalt: "abort"}
		return backend, nil
	}
	plan, err := newDescriptorFontInstallationPlanWithFactory(cfg, 96, "gray", []fontdesc.Descriptor{{Family: "Abort Mono"}}, construct)
	if err != nil {
		t.Fatal(err)
	}
	stages := defaultFontInstallationStages()
	injected := errors.New("abort descriptor context")
	stages.context = func(fontInstallationPlan, fontglyph.Backend, fontInstallationMetrics) (*atlasFontContext, error) {
		return nil, injected
	}
	if prepared, err := prepareFontInstallation(plan, stages); prepared != nil || !errors.Is(err, injected) {
		t.Fatalf("prepared=%v error=%v", prepared, err)
	}
	if backend == nil || backend.closeCalls != 1 {
		t.Fatalf("descriptor candidate close calls = %v", backend)
	}
}

func TestFontInstallationRejectsInvalidMetricsAndIncompleteStages(t *testing.T) {
	backend := &atlasTestBackend{cellW: 0, cellH: 16, baseline: 12}
	plan := testFontInstallationPlan(t, backend)
	if _, err := prepareFontInstallation(plan, defaultFontInstallationStages()); err == nil {
		t.Fatal("invalid metrics were accepted")
	}
	if backend.closeCalls != 1 {
		t.Fatalf("invalid metrics close calls=%d", backend.closeCalls)
	}
	backend = &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}
	plan = testFontInstallationPlan(t, backend)
	stages := defaultFontInstallationStages()
	stages.load = nil
	if _, err := prepareFontInstallation(plan, stages); err == nil {
		t.Fatal("incomplete stages were accepted")
	}
}
