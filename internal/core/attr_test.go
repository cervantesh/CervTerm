package core

import "testing"

func TestAttrHasExplicitBG(t *testing.T) {
	if (Attr{BG: DefaultBG}).HasExplicitBG() {
		t.Fatalf("default background should not count as explicit")
	}
	if !(Attr{BG: RGB{R: 1, G: 2, B: 3}}).HasExplicitBG() {
		t.Fatalf("non-default background should count as explicit")
	}
}

func TestAttrHasExplicitFG(t *testing.T) {
	if (Attr{FG: DefaultFG}).HasExplicitFG() {
		t.Fatalf("default foreground should not count as explicit")
	}
	if !(Attr{FG: RGB{R: 1, G: 2, B: 3}}).HasExplicitFG() {
		t.Fatalf("non-default foreground should count as explicit")
	}
}
