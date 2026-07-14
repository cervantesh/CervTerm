package core

import "testing"

// writeSearchLines writes each string as its own terminal line, scrolling older
// lines into scrollback so the physical-row (global) index space spans both
// history and the live screen.
func writeSearchLines(t *Terminal, lines []string) {
	for _, line := range lines {
		t.CarriageReturn()
		for _, r := range line {
			t.PutRune(r)
		}
		t.NewLine()
	}
}

func TestSearchBackwardBasic(t *testing.T) {
	term := NewTerminal(20, 3)
	writeSearchLines(term, []string{
		"first apple line",
		"second banana line",
		"third cherry line",
		"fourth line",
	})
	total := term.ScrollbackLines() + term.Rows()

	tests := []struct {
		name    string
		query   string
		from    int
		wantOK  bool
		wantCol int
	}{
		{name: "found lowercase", query: "banana", from: total, wantOK: true, wantCol: 7},
		{name: "case insensitive", query: "BANANA", from: total, wantOK: true, wantCol: 7},
		{name: "mixed case content and query", query: "Cherry", from: total, wantOK: true, wantCol: 6},
		{name: "miss", query: "durian", from: total, wantOK: false},
		{name: "empty query", query: "", from: total, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, col, ok := term.SearchBackward(tc.query, tc.from)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && col != tc.wantCol {
				t.Fatalf("col = %d, want %d", col, tc.wantCol)
			}
		})
	}
}

// TestSearchBackwardFromRow verifies the strictly-above semantics used to step
// from one match to the next: searching again from the previous match's row
// finds the earlier occurrence, and a match on the same row is not re-reported.
func TestSearchBackwardFromRow(t *testing.T) {
	term := NewTerminal(20, 3)
	writeSearchLines(term, []string{
		"alpha needle one",
		"beta filler line",
		"gamma needle two",
		"delta tail line",
	})
	total := term.ScrollbackLines() + term.Rows()

	// First search from the bottom finds the lower (later) "needle".
	row1, _, ok := term.SearchBackward("needle", total)
	if !ok {
		t.Fatalf("first search missed")
	}
	// Searching strictly above that row finds the earlier "needle".
	row2, _, ok := term.SearchBackward("needle", row1)
	if !ok {
		t.Fatalf("second search missed")
	}
	if row2 >= row1 {
		t.Fatalf("second match row %d not strictly above first %d", row2, row1)
	}
	// Searching strictly above the earliest match finds nothing more.
	if _, _, ok := term.SearchBackward("needle", row2); ok {
		t.Fatalf("expected no match above earliest occurrence")
	}
}

func TestSearchBackwardUnicode(t *testing.T) {
	term := NewTerminal(20, 3)
	writeSearchLines(term, []string{
		"cafe menu",
		"café résumé",
		"plain line",
	})
	total := term.ScrollbackLines() + term.Rows()

	if _, _, ok := term.SearchBackward("café", total); !ok {
		t.Fatalf("expected to find accented query")
	}
	// Case-insensitive over non-ASCII letters.
	if _, _, ok := term.SearchBackward("RÉSUMÉ", total); !ok {
		t.Fatalf("expected case-insensitive accented match")
	}
}
