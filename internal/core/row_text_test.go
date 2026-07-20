package core

import "testing"

// TestRowTextCanonicalPolicy pins the single canonical row-text rule shared by
// selection/copy and term:line(): skip Rune==0 and WideContinuation padding
// (never turn them into spaces), emit rune+combining, trim trailing blanks.
func TestRowTextCanonicalPolicy(t *testing.T) {
	attr := Attr{FG: DefaultColor(), BG: DefaultColor()}

	cases := []struct {
		name  string
		cells []Cell
		want  string
	}{
		{
			// The bug term:line() used to have: {Rune:0} must be skipped, not
			// rendered as a space. "A", hole, "B" flattens to "AB".
			name:  "interior Rune==0 hole is skipped",
			cells: []Cell{{Rune: 'A', Attr: attr}, {Rune: 0, Attr: attr}, {Rune: 'B', Attr: attr}},
			want:  "AB",
		},
		{
			// A wide glyph occupies its own cell plus a WideContinuation cell; the
			// rune appears exactly once and the continuation contributes nothing.
			name:  "wide glyph emitted once",
			cells: []Cell{{Rune: 0x4E16, Attr: attr}, {WideContinuation: true, Attr: attr}, {Rune: 'x', Attr: attr}},
			want:  string([]rune{0x4E16, 'x'}),
		},
		{
			// Trailing spaces AND trailing Rune==0 padding are both trimmed.
			name:  "trailing blanks trimmed",
			cells: []Cell{{Rune: 'A', Attr: attr}, {Rune: 'B', Attr: attr}, {Rune: ' ', Attr: attr}, {Rune: 0, Attr: attr}},
			want:  "AB",
		},
		{
			// Built from explicit runes so the expectation is decomposed
			// (e + U+0301 + x), matching what writeCellText emits — a source-file
			// precomposed literal would not compare equal.
			name:  "combining marks follow their base rune",
			cells: []Cell{NewCellWithCombining('e', attr, 0x0301), {Rune: 'x', Attr: attr}},
			want:  string([]rune{'e', 0x0301, 'x'}),
		},
		{
			name:  "all blank yields empty string",
			cells: []Cell{{Rune: ' ', Attr: attr}, {Rune: 0, Attr: attr}, {WideContinuation: true, Attr: attr}},
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RowText(tc.cells); got != tc.want {
				t.Fatalf("RowText = %q, want %q", got, tc.want)
			}
		})
	}
}
