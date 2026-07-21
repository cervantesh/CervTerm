package main

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph"
	"cervterm/internal/script"
)

func TestRunDoctorPrintsActionableSections(t *testing.T) {
	path := t.TempDir() + "/cervterm.lua"
	if err := os.WriteFile(path, []byte("return {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-"}); code != 0 {
			t.Fatalf("runDoctor exit code = %d, want 0", code)
		}
	})

	for _, want := range []string{
		"CervTerm doctor",
		"version:",
		"platform:",
		"config:",
		"diagnostics:",
		"environment:",
		"text-gamma: 1.15",
		"text-darken: 0.00",
		"ime-enabled: false",
		"ime-activation: unavailable",
		"accessibility-enabled: false",
		"accessibility-scope: visible",
		"accessibility-activation: unavailable",
		"graphics-kitty-enabled: false",
		"graphics-kitty-activation: dormant",
		"graphics-kitty-limits: encoded-per-pane=8388608 decoded-per-pane=67108864 images-per-pane=256 placements-per-pane=1024 gpu-bytes-per-context=268435456",
		"background-formats: png,jpeg,gif-static",
		"background-budget: cpu=134217728 gpu=134217728",
		"background-surface-capability: runtime-probed",
		"support:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q\n%s", want, output)
		}
	}
}

func TestRunDoctorReportsSubpixelEngine(t *testing.T) {
	path := t.TempDir() + "/cervterm.lua"
	if err := os.WriteFile(path, []byte("return { render = { text_raster = 'subpixel' } }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-"}); code != 0 {
			t.Fatalf("runDoctor exit code = %d, want 0", code)
		}
	})
	if !strings.Contains(output, "text-raster: subpixel") {
		t.Fatalf("doctor output missing subpixel engine\n%s", output)
	}
}

func TestRunDoctorReportsComposedV2AndRedactsSensitiveValues(t *testing.T) {
	path := writeDiagnosticConfig(t, `return {config_version=2,colors={background="#080B12"},shell={env={API_TOKEN="doctor-secret"}},profiles={work={window={opacity=0.8}}},default_profile="work"}`)
	profile := "work"
	options := script.CandidateOptions{Composition: config.CompositionOptions{Selection: config.SelectionOptions{ProfileOverride: &profile}}}
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-", CandidateOptions: options, ContentScale: "not probed in diagnostic mode"}); code != 0 {
			t.Fatalf("runDoctor exit code = %d, want 0", code)
		}
	})
	for _, want := range []string{"schema: authored=2 effective=2", "composed configuration:", `profile: "work" [explicit]`, "shell.env = <redacted>", "pending: unavailable", "last-reload-failure: unavailable", "content-scale: not probed in diagnostic mode"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q\n%s", want, output)
		}
	}
	if strings.Contains(output, "doctor-secret") {
		t.Fatalf("doctor leaked sensitive value\n%s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, reader)
		done <- err
	}()

	fn()

	_ = writer.Close()
	os.Stdout = old
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	return buf.String()
}

func TestRunDoctorReturnsFailureForInvalidComposedConfig(t *testing.T) {
	path := writeDiagnosticConfig(t, `return {config_version=2,unknown_field=true}`)
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-"}); code != 1 {
			t.Fatalf("runDoctor exit code = %d, want 1", code)
		}
	})
	if !strings.Contains(output, "load: error:") {
		t.Fatalf("doctor failure output missing load error\n%s", output)
	}
}

func TestRunDoctorReportsSafeFontsEffectiveFamily(t *testing.T) {
	path := t.TempDir() + "/cervterm.lua"
	if err := os.WriteFile(path, []byte(`return { config_version = 2, font = { family = "Configured Missing Family", descriptors = {{ family = "Configured Descriptor" }}, fallback = {{ family = "Configured Fallback" }}, rules = {{ match = { class = "emoji" }, use = { family = "Configured Rule" } }}, features = { ss01 = 1 }, line_height = 1.5, cell_width = 1.25, baseline_offset = 2, glyph_offset_x = 3, glyph_offset_y = -4 } }`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-", SafeFonts: true}); code != 0 {
			t.Fatalf("runDoctor exit code = %d, want 0", code)
		}
	})
	for _, want := range []string{"safe-fonts: enabled", "font-configured-family: Configured Missing Family", "font-descriptors-suppressed-by-safe-mode: 1", "font-fallback-suppressed-by-safe-mode: 1", "font-rules-suppressed-by-safe-mode: 1", "font-features-suppressed-by-safe-mode: 1", "font-metrics-suppressed-by-safe-mode: true", "font-features: calt=0,clig=0,liga=0", "font-metrics: line-height=1.00 cell-width=1.00 baseline-offset=0.00 glyph-offset-x=0.00 glyph-offset-y=0.00", "font-family: Go Mono"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor safe-font output missing %q\n%s", want, output)
		}
	}
}

func TestRunDoctorProbesFontDescriptors(t *testing.T) {
	path := writeDiagnosticConfig(t, `return {config_version=2,font={family="Legacy",descriptors={{family="Go Mono",weight=400,style="normal",stretch=100,attribute_mode="augment"}},features={liga=0,ss01=1}}}`)
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-"}); code != 0 {
			t.Fatalf("runDoctor exit code = %d, want 0", code)
		}
	})
	for _, want := range []string{"font-features: calt=0,clig=0,liga=0,ss01=1", "font-feature-capability:", "font-descriptors: 1", "font-descriptor[1]: Go Mono weight=400 style=normal stretch=100 mode=augment", `font-style[normal]: family="Go Mono" subfamily="Regular"`, "tier=embedded", "synthetic=none", "font-cell-metrics:", "font-contexts: unavailable (no active frontend; limit=64)", "font-negative-cache: unavailable (no active frontend; limit=8192/context)", "font-parsed-cache: unavailable (diagnostic probe; limits=128 faces/268435456 bytes)", "text-raster:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor descriptor output missing %q\n%s", want, output)
		}
	}
}

func TestRunDoctorProbesFallbackRules(t *testing.T) {
	path := writeDiagnosticConfig(t, `return {config_version=2,font={family="Go Mono",fallback={{family="Go Mono"}},rules={{match={class="box_drawing"},use={family="Go Mono"}}}}}`)
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-"}); code != 0 {
			t.Fatalf("runDoctor exit code = %d, want 0", code)
		}
	})
	for _, want := range []string{"font-descriptors: 0", "font-fallback: 1", "font-rules: 1", "font-content[box-drawing]:", "text-raster:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor fallback/rule output missing %q\n%s", want, output)
		}
	}
}

func TestFontDoctorRedactsResolvedPaths(t *testing.T) {
	const windowsPath = `C:\\Users\\private\\SecretFont.ttf`
	const unixPath = "/home/private/SecretFont-Bold.ttf"
	output := captureStdout(t, func() {
		printResolvedFontDoctor("Configured Family", fontglyph.FontResolution{Found: true, Regular: windowsPath, Bold: unixPath})
	})
	for _, leaked := range []string{windowsPath, unixPath, "SecretFont.ttf", "SecretFont-Bold.ttf"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("font doctor leaked %q in %s", leaked, output)
		}
	}
	if !strings.Contains(output, "font-resolution: system (paths redacted)") || !strings.Contains(output, "regular=true bold=true italic=false bold-italic=false") {
		t.Fatalf("redacted resolution summary missing: %s", output)
	}
}

func TestSyntheticModeFormatting(t *testing.T) {
	cases := map[fontdesc.SyntheticMode]string{
		fontdesc.SyntheticNone:                            "none",
		fontdesc.SyntheticBold:                            "bold",
		fontdesc.SyntheticItalic:                          "italic",
		fontdesc.SyntheticBold | fontdesc.SyntheticItalic: "bold+italic",
	}
	for mode, want := range cases {
		if got := formatSyntheticMode(mode); got != want {
			t.Fatalf("formatSyntheticMode(%d)=%q, want %q", mode, got, want)
		}
	}
}

func TestDoctorReportsConfiguredIMEIntentAndPlatformCapability(t *testing.T) {
	path := t.TempDir() + "/cervterm.lua"
	if err := os.WriteFile(path, []byte(`return {config_version=2,ime={enabled=true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-"}); code != 0 {
			t.Fatalf("runDoctor exit code=%d", code)
		}
	})
	capability := "ime-platform-capability: unsupported"
	if runtime.GOOS == "windows" {
		capability = "ime-platform-capability: windows-native-opt-in"
	}
	for _, want := range []string{"ime-enabled: true", capability, "ime-activation: unavailable (no active frontend in diagnostic mode)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q\n%s", want, output)
		}
	}
}

func TestDoctorReportsConfiguredAccessibilityIntentAndPlatformCapability(t *testing.T) {
	path := t.TempDir() + "/cervterm.lua"
	if err := os.WriteFile(path, []byte(`return {config_version=2,accessibility={enabled=true,scope="visible"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-"}); code != 0 {
			t.Fatalf("runDoctor exit code=%d", code)
		}
	})
	capability := "accessibility-platform-capability: unsupported"
	if runtime.GOOS == "windows" {
		capability = "accessibility-platform-capability: windows-uia-opt-in"
	}
	for _, want := range []string{"accessibility-enabled: true", "accessibility-scope: visible", capability, "accessibility-activation: unavailable (no active frontend in diagnostic mode)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q\n%s", want, output)
		}
	}
	if strings.Contains(output, "HWND") || strings.Contains(output, "provider-token") {
		t.Fatalf("doctor leaked native accessibility details\n%s", output)
	}
}
