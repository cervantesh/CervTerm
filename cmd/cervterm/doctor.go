package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"

	"cervterm/internal/applog"
	backgroundcore "cervterm/internal/background"
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
	fmt.Printf("  ime-enabled: %t\n", report.Config.IME.Enabled)
	if runtime.GOOS == "windows" {
		fmt.Println("  ime-platform-capability: windows-native-opt-in")
	} else {
		fmt.Println("  ime-platform-capability: unsupported")
	}
	fmt.Println("  ime-activation: unavailable (no active frontend in diagnostic mode)")
	fmt.Printf("  accessibility-enabled: %t\n", report.Config.Accessibility.Enabled)
	fmt.Printf("  accessibility-scope: %s\n", report.Config.Accessibility.Scope)
	if runtime.GOOS == "windows" {
		fmt.Println("  accessibility-platform-capability: windows-uia-opt-in")
	} else {
		fmt.Println("  accessibility-platform-capability: unsupported")
	}
	fmt.Println("  accessibility-activation: unavailable (no active frontend in diagnostic mode)")
	fmt.Printf("  graphics-kitty-enabled: %t\n", report.Config.Graphics.Kitty.Enabled)
	fmt.Println("  graphics-kitty-activation: dormant (restart-scoped; runtime wiring not installed)")
	fmt.Printf("  graphics-kitty-limits: encoded-per-pane=%d decoded-per-pane=%d images-per-pane=%d placements-per-pane=%d gpu-bytes-per-context=%d\n", report.Config.Graphics.Limits.EncodedBytesPerPane, report.Config.Graphics.Limits.DecodedBytesPerPane, report.Config.Graphics.Limits.ImageCountPerPane, report.Config.Graphics.Limits.PlacementCountPerPane, report.Config.Graphics.Limits.GPUBytesPerContext)
	fmt.Println("  background-formats: png,jpeg,gif-static")
	fmt.Printf("  background-budget: cpu=%d gpu=%d encoded-per-image=%d encoded-aggregate=%d\n", backgroundcore.MaxAggregateCPUBytes, backgroundcore.MaxAggregateGPUBytes, backgroundcore.MaxEncodedBytesPerImage, backgroundcore.MaxAggregateEncodedBytes)
	fmt.Printf("  background-layers: %d\n", len(report.Config.Background.Layers))
	fmt.Println("  background-surface-capability: runtime-probed (OpenGL frontend supported; headless unavailable)")
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
	if safeFonts && len(report.Config.Font.Features) != 0 {
		fmt.Printf("  font-features-suppressed-by-safe-mode: %d\n", len(report.Config.Font.Features))
	}
	if safeFonts && (report.Config.Font.LineHeight != 1 || report.Config.Font.CellWidth != 1 || report.Config.Font.BaselineOffset != 0 || report.Config.Font.GlyphOffsetX != 0 || report.Config.Font.GlyphOffsetY != 0) {
		fmt.Println("  font-metrics-suppressed-by-safe-mode: true")
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
	features, featureErr := fontdesc.NewFeatureSet(cfg.Font.Ligatures, cfg.Font.Features)
	if featureErr != nil {
		fmt.Printf("  font-features: error: %v\n", featureErr)
		return false
	}
	fmt.Printf("  font-features: %s\n", formatFeatureSet(features))
	metrics, metricErr := fontdesc.NewMetricProjection(cfg.Font.LineHeight, cfg.Font.CellWidth, cfg.Font.BaselineOffset, cfg.Font.GlyphOffsetX, cfg.Font.GlyphOffsetY)
	if metricErr != nil {
		fmt.Printf("  font-metrics: error: %v\n", metricErr)
		return false
	}
	fmt.Printf("  font-metrics: line-height=%.2f cell-width=%.2f baseline-offset=%.2f glyph-offset-x=%.2f glyph-offset-y=%.2f\n", metrics.LineHeight, metrics.CellWidth, metrics.BaselineOffset, metrics.GlyphOffsetX, metrics.GlyphOffsetY)
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
			Features: features.CanonicalBytes(),
			Metrics:  metrics.CanonicalBytes(),
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
		fmt.Println("  font-probe: failed (details redacted; inspect diagnostics log)")
		fmt.Println("  text-raster: go")
	} else {
		fontglyph.ConfigureBackendFeatures(backend, features)
		fmt.Printf("  font-feature-capability: %s\n", fontglyph.BackendFeatureCapability(backend))
		naturalW, naturalH, naturalBaseline := backend.CellMetrics()
		cellW, cellH, baseline := metrics.ProjectCellMetrics(naturalW, naturalH, naturalBaseline)
		fmt.Printf("  font-cell-metrics: natural=%dx%d/%d projected=%dx%d/%d\n", naturalW, naturalH, naturalBaseline, cellW, cellH, baseline)
		printFontStyleDoctor(backend)
		if len(cfg.Font.Fallback) != 0 || len(cfg.Font.Rules) != 0 {
			printFontContentDoctor(backend)
		}
		engine := "go"
		if reporter, ok := backend.(interface{ TextRasterEngine() string }); ok {
			engine = reporter.TextRasterEngine()
		}
		fmt.Printf("  text-raster: %s\n", engine)
		backend.Close()
	}
	fmt.Printf("  font-contexts: unavailable (no active frontend; limit=%d)\n", fontdesc.MaxRetainedContexts)
	fmt.Printf("  font-negative-cache: unavailable (no active frontend; limit=%d/context)\n", fontdesc.MaxNegativeEntries)
	fmt.Printf("  font-parsed-cache: unavailable (diagnostic probe; limits=%d faces/%d bytes)\n", fontdesc.MaxParsedFaces, fontdesc.MaxParsedBytes)
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
		effective.Font.Features = nil
		effective.Font.LineHeight = 1
		effective.Font.CellWidth = 1
		effective.Font.BaselineOffset = 0
		effective.Font.GlyphOffsetX = 0
		effective.Font.GlyphOffsetY = 0
	}
	return effective
}

func formatFeatureSet(features fontdesc.FeatureSet) string {
	entries := features.Entries()
	if len(entries) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(entries))
	for _, feature := range entries {
		parts = append(parts, fmt.Sprintf("%s=%d", feature.Tag, feature.Value))
	}
	return strings.Join(parts, ",")
}

func printFontStyleDoctor(backend fontglyph.Backend) {
	styles := []struct {
		name    string
		request fontdesc.RequestedFaceStyle
	}{
		{"normal", fontdesc.RequestedFaceStyleNormal},
		{"bold", fontdesc.RequestedFaceStyleBold},
		{"italic", fontdesc.RequestedFaceStyleItalic},
		{"bold-italic", fontdesc.RequestedFaceStyleBoldItalic},
	}
	for _, style := range styles {
		face, resolved := fontglyph.DiagnosticStyleFace(backend, style.request)
		if !resolved {
			fmt.Printf("  font-style[%s]: unavailable\n", style.name)
			continue
		}
		printFaceDiagnostic(fmt.Sprintf("font-style[%s]", style.name), face)
	}
}

func printFontContentDoctor(backend fontglyph.Backend) {
	probes := []struct {
		name    string
		content string
	}{
		{"powerline", "\ue0b0"},
		{"nerd-font", "\uf120"},
		{"cjk", "中"},
		{"emoji", "😀"},
		{"box-drawing", "─"},
		{"braille", "⠿"},
		{"symbol", "★"},
	}
	for _, probe := range probes {
		face, resolved := fontglyph.DiagnosticContentFace(backend, fontdesc.RequestedFaceStyleNormal, probe.content)
		if !resolved {
			fmt.Printf("  font-content[%s]: unavailable\n", probe.name)
			continue
		}
		printFaceDiagnostic(fmt.Sprintf("font-content[%s]", probe.name), face)
	}
}

func printFaceDiagnostic(label string, face fontglyph.FaceDiagnostic) {
	metadata := face.Metadata.Normalized()
	fmt.Printf("  %s: family=%q subfamily=%q weight=%d style=%s stretch=%d collection=%d tier=%s source-index=%d synthetic=%s\n",
		label, metadata.Family, metadata.Subfamily, metadata.Weight, metadata.Style, metadata.Stretch, metadata.CollectionIndex, formatSourceTier(face.Tier), face.AuthoredIndex, formatSyntheticMode(face.Synthetic))
}

func formatSourceTier(tier fontdesc.SourceTier) string {
	switch tier {
	case fontdesc.SourceTierRule:
		return "rule"
	case fontdesc.SourceTierPrimary:
		return "primary"
	case fontdesc.SourceTierFallback:
		return "fallback"
	case fontdesc.SourceTierEmbedded:
		return "embedded"
	default:
		return "unknown"
	}
}

func formatSyntheticMode(mode fontdesc.SyntheticMode) string {
	if mode == fontdesc.SyntheticNone {
		return "none"
	}
	parts := make([]string, 0, 2)
	if mode&fontdesc.SyntheticBold != 0 {
		parts = append(parts, "bold")
	}
	if mode&fontdesc.SyntheticItalic != 0 {
		parts = append(parts, "italic")
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, "+")
}

func printFontDoctor(family string) {
	printResolvedFontDoctor(family, fontglyph.ResolveSystemFont(family))
}

func printResolvedFontDoctor(family string, resolution fontglyph.FontResolution) {
	fmt.Printf("  font-family: %s\n", family)
	if !resolution.Found {
		fmt.Println("  font-resolution: embedded fallback")
		fmt.Println("  warning: configured font family was not found")
		return
	}
	fmt.Println("  font-resolution: system (paths redacted)")
	fmt.Printf("  font-style-availability: regular=%t bold=%t italic=%t bold-italic=%t\n", resolution.Regular != "", resolution.Bold != "", resolution.Italic != "", resolution.BoldItalic != "")
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
