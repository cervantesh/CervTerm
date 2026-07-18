package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"

	"cervterm/internal/applog"
	"cervterm/internal/buildinfo"
	"cervterm/internal/config"
	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph"
	"cervterm/internal/script"
)

type doctorOptions struct {
	ConfigPath       string
	LogPath          string
	EmojiWarnings    []string
	ContentScale     string
	CandidateOptions script.CandidateOptions
	SafeFonts        bool
}

func runDoctor(opts doctorOptions) int {
	fmt.Println("CervTerm doctor")
	fmt.Printf("version: %s\n", buildinfo.Info())
	fmt.Printf("platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	if exe, err := os.Executable(); err == nil {
		fmt.Printf("executable: %s\n", exe)
	} else {
		fmt.Printf("executable: unavailable (%v)\n", err)
	}
	if cwd, err := os.Getwd(); err == nil {
		fmt.Printf("working-directory: %s\n", cwd)
	}

	configOK := printConfigDoctor(opts.ConfigPath, opts.CandidateOptions, opts.SafeFonts)
	printLogDoctor(opts.LogPath)
	printEnvironmentDoctor()
	if opts.ContentScale == "" {
		fmt.Println("content-scale: unavailable in headless build")
	} else {
		fmt.Printf("content-scale: %s\n", opts.ContentScale)
	}
	printEmojiDoctor(opts.EmojiWarnings)
	fmt.Println("support: attach this output, the diagnostics log, and screenshots/captures when filing an issue")
	if !configOK {
		return 1
	}
	return 0
}

func printConfigDoctor(configPath string, candidateOptions script.CandidateOptions, safeFonts bool) bool {
	fmt.Println("config:")
	if safeFonts {
		fmt.Println("  safe-fonts: enabled")
	} else {
		fmt.Println("  safe-fonts: disabled")
	}
	if strings.TrimSpace(configPath) != "" {
		fmt.Printf("  override: %s\n", configPath)
	} else if discovered := config.DiscoverPath(); discovered != "" {
		fmt.Printf("  discovered: %s\n", discovered)
	} else {
		fmt.Println("  discovered: none (defaults will be used)")
	}
	fmt.Println("  candidates:")
	for _, candidate := range config.CandidatePaths() {
		status := "missing"
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			status = "present"
		}
		fmt.Printf("    - %s [%s]\n", candidate, status)
	}

	report, cleanup, err := loadConfigDiagnostic(configDiagnosticOptions{ConfigPath: configPath, Candidate: candidateOptions}, false)
	if err != nil {
		fmt.Printf("  load: error: %v\n", err)
		return false
	}
	defer cleanup()
	if report.SourcePath == "" {
		fmt.Println("  load: defaults")
		fmt.Println("  schema: none")
		fmt.Println("  composition: unavailable (no source)")
	} else {
		fmt.Printf("  load: %s\n", report.SourcePath)
		fmt.Printf("  schema: authored=%d effective=2\n", report.AuthoredVersion)
		if report.Composition {
			renderConfigDiagnostic(os.Stdout, report, "  composed configuration:")
		} else {
			fmt.Println("  composition: unavailable (v1 compatibility path)")
		}
	}
	fmt.Println("  pending: unavailable (no active frontend in diagnostic mode)")
	fmt.Println("  last-reload-failure: unavailable (no active frontend in diagnostic mode)")
	if safeFonts && report.Config.Font.Family != "Go Mono" {
		fmt.Printf("  font-configured-family: %s\n", report.Config.Font.Family)
	}
	if safeFonts && len(report.Config.Font.Descriptors) != 0 {
		fmt.Printf("  font-descriptors-suppressed-by-safe-mode: %d\n", len(report.Config.Font.Descriptors))
	}
	if safeFonts && len(report.Config.Font.Fallback) != 0 {
		fmt.Printf("  font-fallback-suppressed-by-safe-mode: %d\n", len(report.Config.Font.Fallback))
	}
	if safeFonts && len(report.Config.Font.Rules) != 0 {
		fmt.Printf("  font-rules-suppressed-by-safe-mode: %d\n", len(report.Config.Font.Rules))
	}
	cfg := effectiveDoctorConfig(report.Config, safeFonts)
	if cfg.Shell.Program == "" {
		fmt.Println("  shell: platform default")
	} else {
		fmt.Printf("  shell: %s %s\n", cfg.Shell.Program, strings.Join(cfg.Shell.Args, " "))
	}
	if cfg.Shell.WorkingDirectory != "" {
		fmt.Printf("  shell-working-directory: %s\n", cfg.Shell.WorkingDirectory)
	}
	fmt.Printf("  text-gamma: %.2f\n", cfg.Render.TextGamma)
	fmt.Printf("  text-darken: %.2f\n", cfg.Render.TextDarken)
	spec := fontglyph.Spec{Family: cfg.Font.Family, Size: cfg.Font.Size, DPI: 96, TextRaster: cfg.Render.TextRaster}
	var backend fontglyph.Backend
	var backendErr error
	advanced := len(cfg.Font.Descriptors) != 0 || len(cfg.Font.Fallback) != 0 || len(cfg.Font.Rules) != 0
	if advanced {
		primary := cfg.Font.Descriptors
		if len(primary) == 0 {
			primary = []fontdesc.Descriptor{{Family: cfg.Font.Family}}
		}
		environment, identityErr := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{
			Descriptors: primary, Fallback: cfg.Font.Fallback, Rules: cfg.Font.Rules, BaseSizeBits: math.Float64bits(cfg.Font.Size), PaneZoomBits: math.Float64bits(1),
			DPI: 96, RasterMode: cfg.Render.TextRaster, GammaBits: math.Float64bits(cfg.Render.TextGamma), DarkeningBits: math.Float64bits(cfg.Render.TextDarken),
		})
		if identityErr != nil {
			backendErr = identityErr
		} else if len(cfg.Font.Fallback) != 0 || len(cfg.Font.Rules) != 0 {
			backend, backendErr = fontglyph.NewFallbackBackend(spec, environment, primary, cfg.Font.Fallback, cfg.Font.Rules)
		} else {
			backend, backendErr = fontglyph.NewDescriptorBackend(spec, environment, primary)
		}
		fmt.Printf("  font-descriptors: %d\n", len(cfg.Font.Descriptors))
		fmt.Printf("  font-fallback: %d\n", len(cfg.Font.Fallback))
		fmt.Printf("  font-rules: %d\n", len(cfg.Font.Rules))
		for index, descriptor := range primary {
			normalized := descriptor.Normalized()
			fmt.Printf("  font-descriptor[%d]: %s weight=%d style=%s stretch=%d mode=%s\n", index+1, normalized.Family, normalized.Weight, normalized.Style, normalized.Stretch, normalized.AttributeMode)
		}
	} else {
		backend, backendErr = fontglyph.NewOpenTypeBackend(spec)
	}
	if backendErr != nil {
		fmt.Printf("  text-raster: go (font probe failed: %v)\n", backendErr)
	} else {
		engine := "go"
		if reporter, ok := backend.(interface{ TextRasterEngine() string }); ok {
			engine = reporter.TextRasterEngine()
		}
		fmt.Printf("  text-raster: %s\n", engine)
		backend.Close()
	}
	if !advanced {
		printFontDoctor(cfg.Font.Family)
	}
	return true
}

func effectiveDoctorConfig(authored config.Config, safeFonts bool) config.Config {
	effective := authored.Clone()
	if safeFonts {
		effective.Font.Family = "Go Mono"
		effective.Font.Descriptors = nil
		effective.Font.Fallback = nil
		effective.Font.Rules = nil
	}
	return effective
}

func printFontDoctor(family string) {
	resolution := fontglyph.ResolveSystemFont(family)
	fmt.Printf("  font-family: %s\n", family)
	if !resolution.Found {
		fmt.Println("  font-resolution: not found (using embedded Go Mono)")
		fmt.Println("  warning: configured font family was not found")
		return
	}
	fmt.Printf("  font-regular: %s\n", resolution.Regular)
	for label, path := range map[string]string{"bold": resolution.Bold, "italic": resolution.Italic, "bold-italic": resolution.BoldItalic} {
		if path != "" {
			fmt.Printf("  font-%s: %s\n", label, path)
		}
	}
}

func printLogDoctor(logPath string) {
	fmt.Println("diagnostics:")
	resolved := applog.ResolvePath(logPath)
	if resolved == "" {
		fmt.Println("  log-file: stderr only")
	} else {
		fmt.Printf("  log-file: %s\n", resolved)
	}
	if env := strings.TrimSpace(os.Getenv(applog.EnvLogFile)); env != "" {
		fmt.Printf("  %s: %s\n", applog.EnvLogFile, env)
	}
}

func printEnvironmentDoctor() {
	fmt.Println("environment:")
	for _, key := range []string{"TERM", "COLORTERM", "SHELL", "ComSpec", "APPDATA", "XDG_CONFIG_HOME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}
}

func printEmojiDoctor(warnings []string) {
	if len(warnings) == 0 {
		fmt.Println("emoji-fonts: no warnings reported by this build")
		return
	}
	fmt.Println("emoji-fonts:")
	for _, warning := range warnings {
		fmt.Printf("  warning: %s\n", warning)
	}
}
