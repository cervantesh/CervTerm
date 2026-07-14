package render

import (
	"testing"

	"cervterm/internal/core"
)

func TestCaptureCopiesTerminalStateWithoutAliasing(t *testing.T) {
	term := core.NewTerminal(4, 2)
	for _, r := range "ab" {
		term.PutRune(r)
	}
	term.SetTitle("demo")
	term.SetCwd(`/work/demo`)

	var snap Snapshot
	Capture(&snap, term)

	if snap.Cols != 4 || snap.Rows != 2 {
		t.Fatalf("unexpected dimensions: %dx%d", snap.Cols, snap.Rows)
	}
	if snap.CursorRow != 0 || snap.CursorCol != 2 {
		t.Fatalf("unexpected cursor: row=%d col=%d", snap.CursorRow, snap.CursorCol)
	}
	if snap.Title != "demo" {
		t.Fatalf("unexpected title: %q", snap.Title)
	}
	if snap.Cwd != "/work/demo" {
		t.Fatalf("unexpected cwd: %q", snap.Cwd)
	}
	if got := string([]rune{snap.Cells[0].Rune, snap.Cells[1].Rune}); got != "ab" {
		t.Fatalf("unexpected cells: %q", got)
	}

	term.SetCursor(0, 0)
	term.PutRune('Z')
	if snap.Cells[0].Rune != 'a' {
		t.Fatalf("snapshot aliases terminal cells; got %q", snap.Cells[0].Rune)
	}
}

func TestCaptureReusesSnapshotBackingStore(t *testing.T) {
	term := core.NewTerminal(8, 3)
	var snap Snapshot
	Capture(&snap, term)
	if len(snap.Cells) != 24 {
		t.Fatalf("unexpected cell count: %d", len(snap.Cells))
	}
	first := &snap.Cells[0]

	term.Resize(8, 3)
	Capture(&snap, term)
	if &snap.Cells[0] != first {
		t.Fatalf("expected capture to reuse backing store for same dimensions")
	}

	term.Resize(10, 3)
	Capture(&snap, term)
	if len(snap.Cells) != 30 {
		t.Fatalf("unexpected resized cell count: %d", len(snap.Cells))
	}
}

func TestCaptureUsesScrolledViewport(t *testing.T) {
	term := core.NewTerminal(5, 2)
	writeLine := func(s string) {
		for _, r := range s {
			term.PutRune(r)
		}
		term.CarriageReturn()
		term.NewLine()
	}
	writeLine("one")
	writeLine("two")
	writeLine("tri")
	term.ScrollViewport(2)

	var snap Snapshot
	Capture(&snap, term)
	got := string([]rune{snap.Cells[0].Rune, snap.Cells[1].Rune, snap.Cells[2].Rune})
	if got != "one" {
		t.Fatalf("expected scrolled first row, got %q", got)
	}
}

func TestCaptureHidesCursorWhenScrolledBack(t *testing.T) {
	term := core.NewTerminal(5, 2)
	writeLine := func(s string) {
		for _, r := range s {
			term.PutRune(r)
		}
		term.CarriageReturn()
		term.NewLine()
	}
	writeLine("one")
	writeLine("two")
	writeLine("tri")

	var snap Snapshot
	Capture(&snap, term)
	if !snap.CursorVisible {
		t.Fatalf("cursor should be visible at bottom viewport")
	}

	term.ScrollViewport(1)
	Capture(&snap, term)
	if snap.CursorVisible {
		t.Fatalf("cursor should be hidden while viewport is scrolled back")
	}

	term.ScrollViewport(-1)
	Capture(&snap, term)
	if !snap.CursorVisible {
		t.Fatalf("cursor should be visible again at bottom viewport")
	}
}

func TestCaptureRespectsTerminalCursorVisibilityMode(t *testing.T) {
	term := core.NewTerminal(5, 2)
	var snap Snapshot

	Capture(&snap, term)
	if !snap.CursorVisible {
		t.Fatalf("cursor should default visible")
	}

	term.SetCursorVisible(false)
	Capture(&snap, term)
	if snap.CursorVisible {
		t.Fatalf("cursor should be hidden when terminal mode hides it")
	}
}

func BenchmarkCaptureReuse(b *testing.B) {
	term := core.NewTerminal(120, 40)
	var snap Snapshot
	Capture(&snap, term)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Capture(&snap, term)
	}
}
