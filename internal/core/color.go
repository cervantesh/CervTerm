package core

// RGB is deliberately toolkit-neutral. Renderers translate it to their native color type.
type RGB struct {
	R, G, B uint8
}

var (
	DefaultFG = RGB{R: 0xE8, G: 0xEA, B: 0xED}
	DefaultBG = RGB{R: 0x0B, G: 0x10, B: 0x1A}
)

// ColorKind identifies how a LogicalColor is resolved to a physical RGB value.
type ColorKind uint8

const (
	ColorDefault ColorKind = iota
	ColorIndexed
	ColorRGB
)

// LogicalColor is a compact, comparable terminal color value. Its zero value is
// the terminal default; indexed colors retain their palette identity rather than
// being flattened to RGB in the VT parser.
type LogicalColor uint32

// Color is an alias for LogicalColor for callers that prefer the shorter name.
type Color = LogicalColor

const (
	colorKindShift = 24
	colorValueMask = 0x00ffffff
)

// DefaultColor returns the logical terminal default. The field containing the
// color (foreground or background) determines which physical default resolves.
func DefaultColor() LogicalColor { return 0 }

// IndexedColor returns a logical xterm/ANSI palette index.
func IndexedColor(index uint8) LogicalColor {
	return LogicalColor(uint32(ColorIndexed)<<colorKindShift | uint32(index))
}

// RGBColor returns a logical literal RGB color.
func RGBColor(rgb RGB) LogicalColor {
	return LogicalColor(uint32(ColorRGB)<<colorKindShift | uint32(rgb.R)<<16 | uint32(rgb.G)<<8 | uint32(rgb.B))
}

// Kind reports the logical color representation.
func (c LogicalColor) Kind() ColorKind {
	switch ColorKind(uint32(c) >> colorKindShift) {
	case ColorIndexed:
		return ColorIndexed
	case ColorRGB:
		return ColorRGB
	default:
		return ColorDefault
	}
}

func (c LogicalColor) IsDefault() bool { return c.Kind() == ColorDefault }
func (c LogicalColor) IsIndexed() bool { return c.Kind() == ColorIndexed }
func (c LogicalColor) IsRGB() bool     { return c.Kind() == ColorRGB }

// Index returns the palette index and whether this is an indexed color.
func (c LogicalColor) Index() (uint8, bool) {
	if !c.IsIndexed() {
		return 0, false
	}
	return uint8(uint32(c) & 0xff), true
}

// RGB returns the literal value and whether this is a logical RGB color.
func (c LogicalColor) RGB() (RGB, bool) {
	if !c.IsRGB() {
		return RGB{}, false
	}
	value := uint32(c) & colorValueMask
	return RGB{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value)}, true
}

var ansi16 = [16]RGB{
	{0x1B, 0x22, 0x32}, {0xFF, 0x5C, 0x8A}, {0x8B, 0xF5, 0x9A}, {0xF8, 0xD8, 0x66},
	{0x7A, 0xA2, 0xFF}, {0xD8, 0x8C, 0xFF}, {0x60, 0xE8, 0xF0}, {0xD8, 0xDE, 0xEA},
	{0x57, 0x62, 0x7A}, {0xFF, 0x7A, 0xA8}, {0xA6, 0xFF, 0xB5}, {0xFF, 0xE6, 0x8A},
	{0x9B, 0xB8, 0xFF}, {0xE5, 0xA7, 0xFF}, {0x90, 0xF4, 0xFF}, {0xFF, 0xFF, 0xFF},
}

// ANSIColors returns the built-in 16-color palette by value.
func ANSIColors() [16]RGB { return ansi16 }

// ColorResolver maps logical colors to physical RGB values for a palette.
type ColorResolver struct {
	DefaultFG RGB
	DefaultBG RGB
	indexed   [256]RGB
}

func NewColorResolver(defaultFG, defaultBG RGB, ansi [16]RGB) ColorResolver {
	resolver := ColorResolver{DefaultFG: defaultFG, DefaultBG: defaultBG}
	for index := range resolver.indexed {
		resolver.indexed[index] = resolveIndexedColor(uint8(index), ansi)
	}
	return resolver
}

// SetIndexed replaces one xterm fallback entry. ANSI indexes remain owned by
// the dedicated 16-color palette and cannot be changed through this method.
func (r *ColorResolver) SetIndexed(index uint8, value RGB) bool {
	if r == nil || index < 16 {
		return false
	}
	r.indexed[index] = value
	return true
}

// IndexedRGB returns the effective physical color for one palette index.
func (r ColorResolver) IndexedRGB(index uint8) RGB { return r.indexed[index] }

func DefaultColorResolver() ColorResolver {
	return NewColorResolver(DefaultFG, DefaultBG, ansi16)
}

func (r ColorResolver) ResolveFG(color LogicalColor) RGB {
	return r.resolve(color, r.DefaultFG)
}

func (r ColorResolver) ResolveBG(color LogicalColor) RGB {
	return r.resolve(color, r.DefaultBG)
}

func (r ColorResolver) resolve(color LogicalColor, defaultColor RGB) RGB {
	switch color.Kind() {
	case ColorIndexed:
		index, _ := color.Index()
		return r.indexed[index]
	case ColorRGB:
		rgb, _ := color.RGB()
		return rgb
	default:
		return defaultColor
	}
}

func resolveIndexedColor(index uint8, ansi [16]RGB) RGB {
	if index < 16 {
		return ansi[index]
	}
	if index <= 231 {
		cubeIndex := int(index) - 16
		levels := [6]uint8{0, 95, 135, 175, 215, 255}
		return RGB{
			R: levels[cubeIndex/36],
			G: levels[(cubeIndex/6)%6],
			B: levels[cubeIndex%6],
		}
	}
	level := uint8(8 + (int(index)-232)*10)
	return RGB{R: level, G: level, B: level}
}

// ANSIColor preserves the existing physical 16-color lookup API.
func ANSIColor(index int) RGB {
	if index < 0 || index >= len(ansi16) {
		return DefaultFG
	}
	return ansi16[index]
}

// ANSI256Color preserves the existing physical xterm 256-color lookup API.
func ANSI256Color(index int) RGB {
	if index < 0 || index > 255 {
		return DefaultFG
	}
	return resolveIndexedColor(uint8(index), ansi16)
}
