package theme

import (
	"fmt"

	"cervterm/internal/core"
)

type Color struct {
	R, G, B uint8
}

func (c Color) Hex() string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

type Palette struct {
	Name       string
	Background Color
	Surface    Color
	Chrome     Color
	Foreground Color
	Muted      Color
	Accent     Color
	ANSI       [16]Color
}

func DefaultPalette() Palette {
	p := Palette{
		Name:       "CervTerm dark",
		Background: Color{0x08, 0x0B, 0x12},
		Surface:    Color{0x10, 0x15, 0x20},
		Chrome:     Color{0x13, 0x1B, 0x2A},
		Foreground: Color{0xE6, 0xE1, 0xD8},
		Muted:      Color{0xA8, 0xB3, 0xC7},
		Accent:     Color{0x60, 0xE8, 0xF0},
	}
	// Derive the ANSI ramp from core (the single source of truth the parser
	// resolves SGR against) instead of re-listing the 16 colors here, so the two
	// can never drift. theme->core is presentation->domain, the correct direction.
	for i := 0; i < 16; i++ {
		c := core.ANSIColor(i)
		p.ANSI[i] = Color{c.R, c.G, c.B}
	}
	return p
}
