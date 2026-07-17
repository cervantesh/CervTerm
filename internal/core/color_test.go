package core

import "testing"

func TestLogicalColorRepresentations(t *testing.T) {
	if got := DefaultColor(); !got.IsDefault() || got.Kind() != ColorDefault {
		t.Fatalf("default color kind = %v", got.Kind())
	}
	indexed := IndexedColor(196)
	if index, ok := indexed.Index(); !ok || index != 196 || indexed.Kind() != ColorIndexed {
		t.Fatalf("indexed color = %#v, index=%d ok=%v", indexed, index, ok)
	}
	literal := RGBColor(RGB{R: 12, G: 34, B: 56})
	if rgb, ok := literal.RGB(); !ok || rgb != (RGB{R: 12, G: 34, B: 56}) || literal.Kind() != ColorRGB {
		t.Fatalf("RGB color = %#v, rgb=%#v ok=%v", literal, rgb, ok)
	}
	if indexed == RGBColor(RGB{R: 255, G: 0, B: 0}) {
		t.Fatal("indexed identity must differ from its current resolved RGB")
	}
}

func TestColorResolver(t *testing.T) {
	ansi := ANSIColors()
	ansi[1] = RGB{R: 1, G: 2, B: 3}
	resolver := NewColorResolver(RGB{R: 9}, RGB{B: 9}, ansi)

	tests := []struct {
		name  string
		color LogicalColor
		fg    RGB
		bg    RGB
	}{
		{"default", DefaultColor(), RGB{R: 9}, RGB{B: 9}},
		{"custom ANSI", IndexedColor(1), RGB{R: 1, G: 2, B: 3}, RGB{R: 1, G: 2, B: 3}},
		{"cube red", IndexedColor(196), RGB{R: 255}, RGB{R: 255}},
		{"cube green", IndexedColor(46), RGB{G: 255}, RGB{G: 255}},
		{"cube blue", IndexedColor(21), RGB{B: 255}, RGB{B: 255}},
		{"grayscale low", IndexedColor(232), RGB{R: 8, G: 8, B: 8}, RGB{R: 8, G: 8, B: 8}},
		{"grayscale high", IndexedColor(255), RGB{R: 238, G: 238, B: 238}, RGB{R: 238, G: 238, B: 238}},
		{"literal", RGBColor(RGB{R: 7, G: 8, B: 9}), RGB{R: 7, G: 8, B: 9}, RGB{R: 7, G: 8, B: 9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolver.ResolveFG(tt.color); got != tt.fg {
				t.Fatalf("ResolveFG = %#v, want %#v", got, tt.fg)
			}
			if got := resolver.ResolveBG(tt.color); got != tt.bg {
				t.Fatalf("ResolveBG = %#v, want %#v", got, tt.bg)
			}
		})
	}
}

func TestANSI256ColorCompatibility(t *testing.T) {
	resolver := DefaultColorResolver()
	for _, index := range []uint8{1, 21, 46, 196, 232, 255} {
		if got, want := ANSI256Color(int(index)), resolver.ResolveFG(IndexedColor(index)); got != want {
			t.Fatalf("ANSI256Color(%d) = %#v, want %#v", index, got, want)
		}
	}
}
