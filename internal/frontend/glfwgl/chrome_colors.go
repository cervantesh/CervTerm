//go:build glfw

package glfwgl

import (
	"image/color"

	"cervterm/internal/config"
)

type chromeColors struct {
	background       color.RGBA
	muted            color.RGBA
	accent           color.RGBA
	split            color.RGBA
	searchMatch      color.RGBA
	error            color.RGBA
	scrollTrack      color.RGBA
	scrollThumb      color.RGBA
	scrollThumbHover color.RGBA
	scrollThumbPress color.RGBA
}

func resolveChromeColors(cfg config.Config) chromeColors {
	return chromeColors{
		background:       configColor(cfg.Colors.ChromeBackground, color.RGBA{0x10, 0x14, 0x1C, 0xF0}),
		muted:            configColor(cfg.Colors.ChromeMuted, color.RGBA{0xA8, 0xB3, 0xC7, 0xFF}),
		accent:           configColor(cfg.Colors.Accent, color.RGBA{0x60, 0xE8, 0xF0, 0xFF}),
		split:            configColor(cfg.Colors.Split, color.RGBA{0x4A, 0x52, 0x63, 0xFF}),
		searchMatch:      configColor(cfg.Colors.SearchMatch, color.RGBA{0x7A, 0x5C, 0x12, 0xFF}),
		error:            configColor(cfg.Colors.Error, color.RGBA{0xD8, 0x72, 0x72, 0xFF}),
		scrollTrack:      configColor(cfg.Scrollbar.TrackColor, color.RGBA{}),
		scrollThumb:      configColor(cfg.Scrollbar.ThumbColor, color.RGBA{}),
		scrollThumbHover: configColor(cfg.Scrollbar.ThumbHoverColor, color.RGBA{}),
		scrollThumbPress: configColor(cfg.Scrollbar.ThumbPressColor, color.RGBA{}),
	}
}
