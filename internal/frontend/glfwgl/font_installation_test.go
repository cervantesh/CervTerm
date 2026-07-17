//go:build glfw

package glfwgl

import (
	"errors"
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
	normal := effectiveStartupConfig(authored, false)
	safe := effectiveStartupConfig(authored, true)
	if normal.Font.Family != "Configured Family" || safe.Font.Family != "Go Mono" {
		t.Fatalf("normal/safe families = %q/%q", normal.Font.Family, safe.Font.Family)
	}
	if authored.Font.Family != "Configured Family" {
		t.Fatal("safe mode mutated authored config")
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
