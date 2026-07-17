//go:build glfw

package glfwgl

import (
	"cervterm/internal/config"
	"cervterm/internal/core"
)

func configuredColorResolver(colors config.ColorsConfig) core.ColorResolver {
	foreground := configColor(colors.Foreground, rgb(core.DefaultFG))
	background := configColor(colors.Background, rgb(core.DefaultBG))
	ansi := core.ANSIColors()
	for index, encoded := range colors.ANSI {
		resolved := configColor(encoded, rgb(ansi[index]))
		ansi[index] = core.RGB{R: resolved.R, G: resolved.G, B: resolved.B}
	}
	return core.NewColorResolver(
		core.RGB{R: foreground.R, G: foreground.G, B: foreground.B},
		core.RGB{R: background.R, G: background.G, B: background.B},
		ansi,
	)
}
