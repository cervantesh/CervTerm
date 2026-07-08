package theme

import "testing"

func TestDefaultPaletteIsRefinedDarkTheme(t *testing.T) {
	p := DefaultPalette()
	if p.Name != "CervTerm dark" {
		t.Fatalf("unexpected palette name: %q", p.Name)
	}
	if !isDark(p.Background) || !isDark(p.Surface) {
		t.Fatalf("expected dark background/surface: %#v %#v", p.Background, p.Surface)
	}
	if contrastDistance(p.Foreground, p.Background) < 360 {
		t.Fatalf("foreground/background contrast too low: %#v on %#v", p.Foreground, p.Background)
	}
	if contrastDistance(p.Accent, p.Background) < 180 {
		t.Fatalf("accent/background contrast too low: %#v on %#v", p.Accent, p.Background)
	}
}

func TestDefaultPaletteHasANSI16Colors(t *testing.T) {
	p := DefaultPalette()
	if len(p.ANSI) != 16 {
		t.Fatalf("expected 16 ANSI colors, got %d", len(p.ANSI))
	}
	if p.ANSI[0] == p.ANSI[7] {
		t.Fatalf("black and white ANSI colors must differ")
	}
	if p.ANSI[1] == p.ANSI[2] {
		t.Fatalf("red and green ANSI colors must differ")
	}
}

func TestDefaultPaletteIsStable(t *testing.T) {
	p := DefaultPalette()
	if p.Background.Hex() != "#080B12" {
		t.Fatalf("background changed unexpectedly: %s", p.Background.Hex())
	}
	if p.Foreground.Hex() != "#E6E1D8" {
		t.Fatalf("foreground changed unexpectedly: %s", p.Foreground.Hex())
	}
	if p.Accent.Hex() != "#60E8F0" {
		t.Fatalf("accent changed unexpectedly: %s", p.Accent.Hex())
	}
}

func isDark(c Color) bool {
	return int(c.R)+int(c.G)+int(c.B) < 160
}

func contrastDistance(a, b Color) int {
	dr := int(a.R) - int(b.R)
	if dr < 0 {
		dr = -dr
	}
	dg := int(a.G) - int(b.G)
	if dg < 0 {
		dg = -dg
	}
	db := int(a.B) - int(b.B)
	if db < 0 {
		db = -db
	}
	return dr + dg + db
}
