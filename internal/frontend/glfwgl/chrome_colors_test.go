//go:build glfw

package glfwgl

import (
	"image/color"
	"testing"

	"cervterm/internal/config"
)

func TestResolveChromeColorsDefaultsAndCustomValues(t *testing.T) {
	defaults := config.Defaults()
	wantDefaults := chromeColors{
		background:       color.RGBA{0x10, 0x14, 0x1C, 0xF0},
		muted:            color.RGBA{0xA8, 0xB3, 0xC7, 0xFF},
		accent:           color.RGBA{0x60, 0xE8, 0xF0, 0xFF},
		split:            color.RGBA{0x4A, 0x52, 0x63, 0xFF},
		searchMatch:      color.RGBA{0x7A, 0x5C, 0x12, 0xFF},
		error:            color.RGBA{0xD8, 0x72, 0x72, 0xFF},
		scrollTrack:      color.RGBA{0x10, 0x17, 0x22, 0x66},
		scrollThumb:      color.RGBA{0x60, 0xE8, 0xF0, 0xCC},
		scrollThumbHover: color.RGBA{0x7C, 0xF4, 0xF9, 0xE6},
		scrollThumbPress: color.RGBA{0xB6, 0xFA, 0xFF, 0xFF},
	}
	if got := resolveChromeColors(defaults); got != wantDefaults {
		t.Fatalf("default chrome colors = %#v, want %#v", got, wantDefaults)
	}

	custom := defaults
	custom.Colors.ChromeBackground = "#01020304"
	custom.Colors.ChromeMuted = "#11121314"
	custom.Colors.Accent = "#21222324"
	custom.Colors.Split = "#31323334"
	custom.Colors.SearchMatch = "#41424344"
	custom.Colors.Error = "#51525354"
	custom.Scrollbar.TrackColor = "#61626364"
	custom.Scrollbar.ThumbColor = "#71727374"
	custom.Scrollbar.ThumbHoverColor = "#81828384"
	custom.Scrollbar.ThumbPressColor = "#91929394"
	wantCustom := chromeColors{
		background:       color.RGBA{0x01, 0x02, 0x03, 0x04},
		muted:            color.RGBA{0x11, 0x12, 0x13, 0x14},
		accent:           color.RGBA{0x21, 0x22, 0x23, 0x24},
		split:            color.RGBA{0x31, 0x32, 0x33, 0x34},
		searchMatch:      color.RGBA{0x41, 0x42, 0x43, 0x44},
		error:            color.RGBA{0x51, 0x52, 0x53, 0x54},
		scrollTrack:      color.RGBA{0x61, 0x62, 0x63, 0x64},
		scrollThumb:      color.RGBA{0x71, 0x72, 0x73, 0x74},
		scrollThumbHover: color.RGBA{0x81, 0x82, 0x83, 0x84},
		scrollThumbPress: color.RGBA{0x91, 0x92, 0x93, 0x94},
	}
	if got := resolveChromeColors(custom); got != wantCustom {
		t.Fatalf("custom chrome colors = %#v, want %#v", got, wantCustom)
	}
}
