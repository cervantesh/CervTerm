package core

import "testing"

func TestPaletteOverridesMutateQueryResetAndRetainAcrossBaseUpdate(t *testing.T) {
	term := NewTerminal(4, 2)
	original := term.PaletteBase()
	if got := term.EffectivePaletteIndex(196); got != ANSI256Color(196) {
		t.Fatalf("initial index 196 = %#v", got)
	}

	term.SetPaletteIndex(196, RGB{R: 1, G: 2, B: 3})
	term.SetPaletteFG(RGB{R: 4, G: 5, B: 6})
	term.SetPaletteBG(RGB{R: 7, G: 8, B: 9})
	overrides := term.PaletteOverrides()
	if !overrides.HasIndexed(196) || !overrides.FGSet || !overrides.BGSet || overrides.Generation != 3 {
		t.Fatalf("overrides = %#v", overrides)
	}

	next := original
	next.FG = RGB{R: 10}
	next.BG = RGB{B: 11}
	next.Indexed[196] = RGB{G: 12}
	term.SetPaletteBase(next)
	if after := term.PaletteOverrides(); after != overrides {
		t.Fatal("base update changed overrides or generation")
	}
	if got := term.EffectivePaletteIndex(196); got != (RGB{R: 1, G: 2, B: 3}) {
		t.Fatalf("override did not remain effective: %#v", got)
	}

	term.ResetPaletteIndex(196)
	term.ResetPaletteFG()
	term.ResetPaletteBG()
	if got := term.EffectivePaletteIndex(196); got != next.Indexed[196] {
		t.Fatalf("reset did not reveal new indexed base: %#v", got)
	}
	if term.EffectivePaletteFG() != next.FG || term.EffectivePaletteBG() != next.BG {
		t.Fatal("default reset did not reveal new base")
	}
	if term.PaletteOverrides().Generation != 6 {
		t.Fatalf("generation = %d, want 6", term.PaletteOverrides().Generation)
	}
}

func TestPaletteResolverPrecedenceAndTruecolorInvariant(t *testing.T) {
	base := DefaultPaletteBase()
	overrides := PaletteOverrides{FG: RGB{R: 1}, BG: RGB{G: 2}, FGSet: true, BGSet: true}
	overrides.Indexed[1] = RGB{B: 3}
	overrides.IndexedSet[0] = 1 << 1
	resolver := overrides.ColorResolver(base)
	if got := resolver.ResolveFG(DefaultColor()); got != overrides.FG {
		t.Fatalf("default foreground = %#v", got)
	}
	if got := resolver.ResolveBG(DefaultColor()); got != overrides.BG {
		t.Fatalf("default background = %#v", got)
	}
	if got := resolver.ResolveFG(IndexedColor(1)); got != overrides.Indexed[1] {
		t.Fatalf("indexed color = %#v", got)
	}
	literal := RGB{R: 9, G: 8, B: 7}
	if got := resolver.ResolveFG(RGBColor(literal)); got != literal {
		t.Fatalf("truecolor changed = %#v", got)
	}
}

func TestResetAllPaletteIndexesBumpsGenerationOnce(t *testing.T) {
	term := NewTerminal(2, 1)
	term.SetPaletteIndex(1, RGB{R: 1})
	term.SetPaletteIndex(200, RGB{G: 2})
	before := term.PaletteOverrides().Generation
	term.ResetPaletteIndexes()
	after := term.PaletteOverrides()
	if after.IndexedSet != [4]uint64{} || after.Generation != before+1 {
		t.Fatalf("reset all = %#v", after)
	}
}
