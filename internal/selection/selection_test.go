package selection

import (
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/render"
)

func snapshotFromLines(cols int, lines ...string) render.Snapshot {
	term := core.NewTerminal(cols, len(lines))
	for row, line := range lines {
		term.SetCursor(row, 0)
		for _, r := range line {
			term.PutRune(r)
		}
	}
	var snap render.Snapshot
	render.Capture(&snap, term)
	return snap
}

func TestNormalizeOrdersEndpoints(t *testing.T) {
	r := Normalize(Range{Start: Point{Row: 2, Col: 4}, End: Point{Row: 1, Col: 3}})
	if r.Start != (Point{Row: 1, Col: 3}) || r.End != (Point{Row: 2, Col: 4}) {
		t.Fatalf("unexpected normalized range: %#v", r)
	}
}

func TestContainsInclusiveEndpoints(t *testing.T) {
	r := Normalize(Range{Start: Point{Row: 0, Col: 1}, End: Point{Row: 1, Col: 2}})
	for _, p := range []Point{{0, 1}, {0, 5}, {1, 0}, {1, 2}} {
		if !Contains(r, p) {
			t.Fatalf("expected %#v to be selected", p)
		}
	}
	for _, p := range []Point{{0, 0}, {1, 3}, {2, 0}} {
		if Contains(r, p) {
			t.Fatalf("expected %#v to be outside selection", p)
		}
	}
}

func TestTextSingleLineTrimsOnlySelectionEnd(t *testing.T) {
	snap := snapshotFromLines(12, "hello world")
	got := Text(snap, Range{Start: Point{Row: 0, Col: 1}, End: Point{Row: 0, Col: 4}})
	if got != "ello" {
		t.Fatalf("selection text mismatch: %q", got)
	}
}

func TestTextMultiLineTrimsTrailingSpaces(t *testing.T) {
	snap := snapshotFromLines(8, "alpha", "beta", "gamma")
	got := Text(snap, Range{Start: Point{Row: 0, Col: 2}, End: Point{Row: 2, Col: 1}})
	want := "pha\nbeta\nga"
	if got != want {
		t.Fatalf("selection text mismatch: want %q got %q", want, got)
	}
}

func TestTextClampsOutOfBounds(t *testing.T) {
	snap := snapshotFromLines(4, "abcd")
	got := Text(snap, Range{Start: Point{Row: -3, Col: -2}, End: Point{Row: 9, Col: 9}})
	if got != "abcd" {
		t.Fatalf("clamped selection mismatch: %q", got)
	}
}

func TestTextSkipsWideContinuationAndPreservesCombining(t *testing.T) {
	snap := snapshotFromLines(8, "A好e\u0301Z")
	got := Text(snap, Range{Start: Point{Row: 0, Col: 0}, End: Point{Row: 0, Col: 5}})
	if got != "A好e\u0301Z" {
		t.Fatalf("unicode selection mismatch: %q", got)
	}
}
