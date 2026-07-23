package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cervterm/internal/config"
)

type supportMatrixDocument struct {
	Features []supportMatrixFeature `json:"features"`
}

type supportMatrixFeature struct {
	ID                  string `json:"id"`
	Status              string `json:"status"`
	Platform            string `json:"platform"`
	DefaultEnabled      *bool  `json:"default_enabled"`
	SupportClaim        string `json:"support_claim"`
	ManualQualification string `json:"manual_qualification"`
}

func TestDoctorCapabilitiesMatchSupportMatrix(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "parity-support-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document supportMatrixDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	features := make(map[string]supportMatrixFeature, len(document.Features))
	for _, feature := range document.Features {
		if _, duplicate := features[feature.ID]; duplicate {
			t.Fatalf("duplicate support matrix id %q", feature.ID)
		}
		features[feature.ID] = feature
	}
	for _, capability := range doctorCapabilities(config.Defaults()) {
		feature, ok := features[capability.ID]
		if !ok {
			t.Fatalf("doctor capability %q is absent from support matrix", capability.ID)
		}
		if feature.Status != capability.Status || feature.Platform != capability.Platform || feature.ManualQualification != capability.ManualQualification || feature.SupportClaim != capability.SupportClaim {
			t.Fatalf("capability %q drift: doctor=%#v matrix=%#v", capability.ID, capability, feature)
		}
		if capability.DefaultEnabled != nil {
			if feature.DefaultEnabled == nil || *feature.DefaultEnabled != *capability.DefaultEnabled {
				t.Fatalf("capability %q default drift: doctor=%v matrix=%v", capability.ID, capability.DefaultEnabled, feature.DefaultEnabled)
			}
		}
	}
}

func TestDoctorCapabilityProjectionIsValueFreeAndNotProbed(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "SECRET_PROGRAM"
	cfg.Shell.Args = []string{"SECRET_ARGUMENT"}
	cfg.Shell.WorkingDirectory = "SECRET_DIRECTORY"
	cfg.Shell.Env = map[string]string{"SECRET_ENV": "SECRET_VALUE"}
	output := captureStdout(t, func() { printSupportDoctor(cfg) })
	for _, forbidden := range []string{"SECRET_PROGRAM", "SECRET_ARGUMENT", "SECRET_DIRECTORY", "SECRET_ENV", "SECRET_VALUE"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("capability projection leaked %q: %s", forbidden, output)
		}
	}
	if !strings.Contains(output, "  capabilities:\n") {
		t.Fatalf("capability projection is not nested under config: %s", output)
	}
	for _, capability := range doctorCapabilities(cfg) {
		linePrefix := "  " + capability.ID + ":"
		if !strings.Contains(output, linePrefix) || !strings.Contains(output, "activation=not-probed") {
			t.Fatalf("capability output missing detached activation state for %q: %s", capability.ID, output)
		}
	}
}

func TestDoctorCapabilityBuildAvailabilityIsStatic(t *testing.T) {
	capabilities := doctorCapabilities(config.Defaults())
	byID := make(map[string]doctorCapability, len(capabilities))
	for _, capability := range capabilities {
		byID[capability.ID] = capability
	}
	if doctorGLFWBuild {
		if byID["graphics.kitty"].BuildAvailability != "glfw-opengl" {
			t.Fatalf("GLFW image availability=%q", byID["graphics.kitty"].BuildAvailability)
		}
	} else if byID["graphics.kitty"].BuildAvailability != "unavailable-headless" {
		t.Fatalf("headless image availability=%q", byID["graphics.kitty"].BuildAvailability)
	}
	wantIME := "unavailable-headless"
	if doctorGLFWBuild {
		wantIME = "unavailable-platform"
		if runtime.GOOS == "windows" {
			wantIME = "windows-native"
		}
	}
	if byID["input.ime_preedit"].BuildAvailability != wantIME {
		t.Fatalf("IME availability=%q want %q", byID["input.ime_preedit"].BuildAvailability, wantIME)
	}
}
