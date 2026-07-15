package core

import "testing"

// TestCursorStyleShapeAndBlink pins the DECSCUSR mapping the renderer relies on:
// each 1..6 style selects a shape and forces blink on/off, while style 0 (and any
// unrecognized value) carries neither, so the configured cursor is honored.
func TestCursorStyleShapeAndBlink(t *testing.T) {
	cases := []struct {
		style     CursorStyle
		wantShape CursorShape
		wantBlink bool
		wantOK    bool
	}{
		{CursorStyleDefault, CursorShapeDefault, false, false},
		{CursorStyleBlinkingBlock, CursorShapeBlock, true, true},
		{CursorStyleSteadyBlock, CursorShapeBlock, false, true},
		{CursorStyleBlinkingUnderline, CursorShapeUnderline, true, true},
		{CursorStyleSteadyUnderline, CursorShapeUnderline, false, true},
		{CursorStyleBlinkingBar, CursorShapeBar, true, true},
		{CursorStyleSteadyBar, CursorShapeBar, false, true},
		{CursorStyle(99), CursorShapeDefault, false, false}, // unrecognized -> honor config
	}
	for _, tc := range cases {
		if got := tc.style.Shape(); got != tc.wantShape {
			t.Errorf("CursorStyle(%d).Shape() = %d, want %d", tc.style, got, tc.wantShape)
		}
		blink, ok := tc.style.Blink()
		if blink != tc.wantBlink || ok != tc.wantOK {
			t.Errorf("CursorStyle(%d).Blink() = (%t, %t), want (%t, %t)", tc.style, blink, ok, tc.wantBlink, tc.wantOK)
		}
	}
}
