package core

// RGB is deliberately toolkit-neutral. Renderers translate it to their native color type.
type RGB struct {
	R, G, B uint8
}

var (
	DefaultFG = RGB{R: 0xE8, G: 0xEA, B: 0xED}
	DefaultBG = RGB{R: 0x0B, G: 0x10, B: 0x1A}
)

var ansi16 = [16]RGB{
	{0x1B, 0x22, 0x32}, {0xFF, 0x5C, 0x8A}, {0x8B, 0xF5, 0x9A}, {0xF8, 0xD8, 0x66},
	{0x7A, 0xA2, 0xFF}, {0xD8, 0x8C, 0xFF}, {0x60, 0xE8, 0xF0}, {0xD8, 0xDE, 0xEA},
	{0x57, 0x62, 0x7A}, {0xFF, 0x7A, 0xA8}, {0xA6, 0xFF, 0xB5}, {0xFF, 0xE6, 0x8A},
	{0x9B, 0xB8, 0xFF}, {0xE5, 0xA7, 0xFF}, {0x90, 0xF4, 0xFF}, {0xFF, 0xFF, 0xFF},
}

func ANSIColor(index int) RGB {
	if index < 0 || index >= len(ansi16) {
		return DefaultFG
	}
	return ansi16[index]
}

func ANSI256Color(index int) RGB {
	if index < 0 {
		return DefaultFG
	}
	if index < 16 {
		return ANSIColor(index)
	}
	if index <= 231 {
		index -= 16
		levels := [6]uint8{0, 95, 135, 175, 215, 255}
		return RGB{
			R: levels[index/36],
			G: levels[(index/6)%6],
			B: levels[index%6],
		}
	}
	if index <= 255 {
		level := uint8(8 + (index-232)*10)
		return RGB{R: level, G: level, B: level}
	}
	return DefaultFG
}
