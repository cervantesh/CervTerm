package core

import "testing"

// TestViewportTopGlobalRow pins the canonical scrollback->viewport arithmetic:
// the top visible global row is scrollbackRows - displayOffset. app_search and
// app_draw derive their viewport rows from this instead of re-computing it.
func TestViewportTopGlobalRow(t *testing.T) {
	term := NewTerminal(4, 3)
	// Build 5 scrollback rows so the viewport can be scrolled back over history.
	for i := 0; i < 5; i++ {
		term.appendScrollbackLine(make([]Cell, 4), false)
	}
	if got := term.ScrollbackLines(); got != 5 {
		t.Fatalf("ScrollbackLines = %d, want 5", got)
	}

	// displayOffset == 0 (live tail): top == scrollbackRows.
	if term.DisplayOffset() != 0 {
		t.Fatalf("DisplayOffset = %d, want 0", term.DisplayOffset())
	}
	if got := term.ViewportTopGlobalRow(); got != 5 {
		t.Fatalf("ViewportTopGlobalRow at offset 0 = %d, want 5", got)
	}

	// displayOffset > 0: scrolled back 2 rows into history.
	if !term.ScrollViewport(2) {
		t.Fatal("expected ScrollViewport(2) to move the viewport")
	}
	if term.DisplayOffset() != 2 {
		t.Fatalf("DisplayOffset = %d, want 2", term.DisplayOffset())
	}
	if got := term.ViewportTopGlobalRow(); got != 3 {
		t.Fatalf("ViewportTopGlobalRow at offset 2 = %d, want 3", got)
	}

	// displayOffset == scrollbackRows (fully scrolled back): the oldest scrollback
	// row sits at the top, so the top global row is 0. This is the max offset.
	if !term.ScrollViewport(3) { // 2 -> 5 == scrollbackRows
		t.Fatal("expected ScrollViewport(3) to reach the top of history")
	}
	if term.DisplayOffset() != 5 {
		t.Fatalf("DisplayOffset = %d, want 5 (max)", term.DisplayOffset())
	}
	if got := term.ViewportTopGlobalRow(); got != 0 {
		t.Fatalf("ViewportTopGlobalRow at max offset = %d, want 0", got)
	}
	// Bounds at the top: global row 0 is viewport row 0; global row rows(=3) is
	// the first row past the visible window.
	if row, ok := term.GlobalRowToViewport(0); !ok || row != 0 {
		t.Fatalf("GlobalRowToViewport(0) at max offset = (%d, %t), want (0, true)", row, ok)
	}
	if _, ok := term.GlobalRowToViewport(3); ok {
		t.Fatal("global row 3 should be off-screen at max offset (viewport shows 0,1,2)")
	}
}

// TestGlobalRowToViewport pins the inverse mapping and its out-of-window guard.
func TestGlobalRowToViewport(t *testing.T) {
	term := NewTerminal(4, 3)
	for i := 0; i < 5; i++ {
		term.appendScrollbackLine(make([]Cell, 4), false)
	}
	term.ScrollViewport(2) // top global row == 3, viewport shows global rows 3,4,5

	cases := []struct {
		name    string
		global  int
		wantRow int
		wantOK  bool
	}{
		{"top visible row maps to viewport 0", 3, 0, true},
		{"middle visible row", 4, 1, true},
		{"bottom visible row maps to viewport rows-1", 5, 2, true},
		{"row above the window is not visible", 2, 0, false},
		{"row below the window is not visible", 6, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row, ok := term.GlobalRowToViewport(tc.global)
			if ok != tc.wantOK || row != tc.wantRow {
				t.Fatalf("GlobalRowToViewport(%d) = (%d, %t), want (%d, %t)", tc.global, row, ok, tc.wantRow, tc.wantOK)
			}
		})
	}

	// displayOffset == 0 case: viewport shows global rows 5,6,7 (the live screen).
	term.ScrollViewport(-2)
	if term.DisplayOffset() != 0 {
		t.Fatalf("DisplayOffset = %d, want 0", term.DisplayOffset())
	}
	if row, ok := term.GlobalRowToViewport(5); !ok || row != 0 {
		t.Fatalf("GlobalRowToViewport(5) at offset 0 = (%d, %t), want (0, true)", row, ok)
	}
	if _, ok := term.GlobalRowToViewport(4); ok {
		t.Fatal("global row 4 should be off-screen at offset 0")
	}
}
