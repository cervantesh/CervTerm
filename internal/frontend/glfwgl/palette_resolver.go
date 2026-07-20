//go:build glfw

package glfwgl

import (
	"image/color"

	"cervterm/internal/config"
	"cervterm/internal/core"
)

func configuredPaletteBase(colors config.ColorsConfig) core.PaletteBase {
	base := core.DefaultPaletteBase()
	foreground := configColor(colors.Foreground, rgb(base.FG))
	background := configColor(colors.Background, rgb(base.BG))
	base.FG = core.RGB{R: foreground.R, G: foreground.G, B: foreground.B}
	base.BG = core.RGB{R: background.R, G: background.G, B: background.B}
	for index, encoded := range colors.ANSI {
		resolved := configColor(encoded, rgb(base.Indexed[index]))
		base.Indexed[index] = core.RGB{R: resolved.R, G: resolved.G, B: resolved.B}
	}
	for slot, encoded := range colors.IndexedColors {
		if encoded == "" {
			continue
		}
		index := slot + 16
		resolved := configColor(encoded, rgb(base.Indexed[index]))
		base.Indexed[index] = core.RGB{R: resolved.R, G: resolved.G, B: resolved.B}
	}
	return base
}

func configuredColorResolver(colors config.ColorsConfig) core.ColorResolver {
	return configuredPaletteBase(colors).ColorResolver()
}

func panePaletteBackground(configured color.RGBA, palette core.PaletteBase, overrides core.PaletteOverrides) color.RGBA {
	if !overrides.BGSet {
		return configured
	}
	return color.RGBA{R: palette.BG.R, G: palette.BG.G, B: palette.BG.B, A: configured.A}
}
