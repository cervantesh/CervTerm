//go:build glfw

package glfwgl

import (
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
