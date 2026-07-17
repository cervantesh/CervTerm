package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"cervterm/internal/config"
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
	if err := os.WriteFile(path, []byte(`return { font = { family = "Configured Missing Family" } }`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if code := runDoctor(doctorOptions{ConfigPath: path, LogPath: "-", SafeFonts: true}); code != 0 {
			t.Fatalf("runDoctor exit code = %d, want 0", code)
		}
	})
	for _, want := range []string{"safe-fonts: enabled", "font-configured-family: Configured Missing Family", "font-family: Go Mono"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor safe-font output missing %q\n%s", want, output)
		}
	}
}
