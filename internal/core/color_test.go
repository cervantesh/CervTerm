package core

import "testing"

func TestANSI256Color(t *testing.T) {
	if got := ANSI256Color(1); got != ANSIColor(1) {
		t.Fatalf("first 16 colors should match ANSI16, got %#v", got)
	}
	if got, want := ANSI256Color(196), (RGB{R: 255, G: 0, B: 0}); got != want {
		t.Fatalf("color cube red mismatch: want %#v got %#v", want, got)
	}
	if got, want := ANSI256Color(46), (RGB{R: 0, G: 255, B: 0}); got != want {
		t.Fatalf("color cube green mismatch: want %#v got %#v", want, got)
	}
	if got, want := ANSI256Color(21), (RGB{R: 0, G: 0, B: 255}); got != want {
		t.Fatalf("color cube blue mismatch: want %#v got %#v", want, got)
	}
	if got, want := ANSI256Color(232), (RGB{R: 8, G: 8, B: 8}); got != want {
		t.Fatalf("grayscale low mismatch: want %#v got %#v", want, got)
	}
	if got, want := ANSI256Color(255), (RGB{R: 238, G: 238, B: 238}); got != want {
		t.Fatalf("grayscale high mismatch: want %#v got %#v", want, got)
	}
}
