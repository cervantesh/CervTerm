//go:build glfw

package glfwgl

import (
	"image/color"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/core"
)

func TestConfiguredPaletteReprojectsLogicalCellsWithoutReparse(t *testing.T) {
	first := config.Defaults().Colors
	second := first
	first.ANSI[1] = "#112233"
	second.ANSI[1] = "#AABBCC"
	logical := core.IndexedColor(1)
	if got := configuredColorResolver(first).ResolveFG(logical); got != (core.RGB{R: 0x11, G: 0x22, B: 0x33}) {
		t.Fatalf("first palette resolve = %#v", got)
	}
	if got := configuredColorResolver(second).ResolveFG(logical); got != (core.RGB{R: 0xAA, G: 0xBB, B: 0xCC}) {
		t.Fatalf("second palette resolve = %#v", got)
	}
	truecolor := core.RGBColor(core.RGB{R: 7, G: 8, B: 9})
	if got := configuredColorResolver(second).ResolveFG(truecolor); got != (core.RGB{R: 7, G: 8, B: 9}) {
		t.Fatalf("truecolor changed with palette = %#v", got)
	}
}

func TestConfiguredPaletteUsesConfiguredLogicalDefaults(t *testing.T) {
	colors := config.Defaults().Colors
	colors.Foreground = "#010203"
	colors.Background = "#040506"
	resolver := configuredColorResolver(colors)
	if got := resolver.ResolveFG(core.DefaultColor()); got != (core.RGB{R: 1, G: 2, B: 3}) {
		t.Fatalf("default foreground = %#v", got)
	}
	if got := resolver.ResolveBG(core.DefaultColor()); got != (core.RGB{R: 4, G: 5, B: 6}) {
		t.Fatalf("default background = %#v", got)
	}
}

func TestConfiguredSparseIndexedOverridesPreserveFallbacks(t *testing.T) {
	first := config.Defaults().Colors
	second := first
	if err := first.IndexedColors.Set(196, "#102030"); err != nil {
		t.Fatal(err)
	}
	if err := second.IndexedColors.Set(196, "#A0B0C0"); err != nil {
		t.Fatal(err)
	}
	logical := core.IndexedColor(196)
	if got := configuredColorResolver(first).ResolveFG(logical); got != (core.RGB{R: 0x10, G: 0x20, B: 0x30}) {
		t.Fatalf("first indexed override = %#v", got)
	}
	if got := configuredColorResolver(second).ResolveFG(logical); got != (core.RGB{R: 0xA0, G: 0xB0, B: 0xC0}) {
		t.Fatalf("second indexed override = %#v", got)
	}
	fallback := configuredColorResolver(first).ResolveFG(core.IndexedColor(195))
	if fallback != core.ANSI256Color(195) {
		t.Fatalf("neighbor fallback = %#v, want %#v", fallback, core.ANSI256Color(195))
	}
	if got := configuredColorResolver(first).ResolveFG(core.IndexedColor(1)); got != core.ANSIColor(1) {
		t.Fatalf("ANSI index changed = %#v", got)
	}
}

func TestPaneOSCOverridesLayerAboveReloadedConfiguredPalette(t *testing.T) {
	first := config.Defaults().Colors
	first.Foreground = "#010203"
	first.Background = "#040506"
	first.ANSI[1] = "#112233"
	second := first
	second.Foreground = "#A1A2A3"
	second.Background = "#B1B2B3"
	second.ANSI[1] = "#C1C2C3"
	overrides := core.PaletteOverrides{
		FG: core.RGB{R: 9, G: 8, B: 7}, FGSet: true,
		BG: core.RGB{R: 6, G: 5, B: 4}, BGSet: true,
	}
	overrides.Indexed[1] = core.RGB{R: 3, G: 2, B: 1}
	overrides.IndexedSet[0] = 1 << 1
	for _, colors := range []config.ColorsConfig{first, second} {
		base := configuredPaletteBase(colors)
		resolver := overrides.ColorResolver(base)
		if got := resolver.ResolveFG(core.DefaultColor()); got != overrides.FG {
			t.Fatalf("OSC foreground = %#v", got)
		}
		if got := resolver.ResolveBG(core.DefaultColor()); got != overrides.BG {
			t.Fatalf("OSC background = %#v", got)
		}
		if got := resolver.ResolveFG(core.IndexedColor(1)); got != overrides.Indexed[1] {
			t.Fatalf("OSC indexed = %#v", got)
		}
		truecolor := core.RGB{R: 0xDE, G: 0xAD, B: 0xBE}
		if got := resolver.ResolveFG(core.RGBColor(truecolor)); got != truecolor {
			t.Fatalf("truecolor changed = %#v", got)
		}
	}
	reset := core.PaletteOverrides{}
	if got := reset.ColorResolver(configuredPaletteBase(second)).ResolveFG(core.DefaultColor()); got != (core.RGB{R: 0xA1, G: 0xA2, B: 0xA3}) {
		t.Fatalf("reset did not reveal reloaded base = %#v", got)
	}
}

func TestPaneOSCBackgroundPreservesConfiguredAlpha(t *testing.T) {
	configured := color.RGBA{R: 1, G: 2, B: 3, A: 0x80}
	base := core.DefaultPaletteBase()
	overrides := core.PaletteOverrides{}
	if got := panePaletteBackground(configured, base, overrides); got != configured {
		t.Fatalf("background without override = %#v", got)
	}
	overrides.BGSet = true
	overrides.BG = core.RGB{R: 0xAA, G: 0xBB, B: 0xCC}
	palette := overrides.Apply(base)
	if got, want := panePaletteBackground(configured, palette, overrides), (color.RGBA{R: 0xAA, G: 0xBB, B: 0xCC, A: 0x80}); got != want {
		t.Fatalf("OSC background = %#v, want %#v", got, want)
	}
	if got, want := applyOpacity(panePaletteBackground(configured, palette, overrides), 0.5), (color.RGBA{R: 0xAA, G: 0xBB, B: 0xCC, A: 0x40}); got != want {
		t.Fatalf("OSC background opacity = %#v, want %#v", got, want)
	}
}
