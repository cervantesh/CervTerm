package core

import (
	"strings"
	"testing"
)

// TestReflowThroughNarrowPreservesInteriorSpaces pins the bug seen when zooming
// into columnar output (dir/ls): rewrapping a wide, space-aligned line through a
// narrow width and back must not collapse the interior alignment spaces. The
// terminal is tall enough that the line never spills into scrollback (that seam
// interaction is a separate concern).
func TestReflowThroughNarrowPreservesInteriorSpaces(t *testing.T) {
	term := NewTerminal(80, 24)
	line := "07/11/2026  09:09 AM    <DIR>          internal"
	for _, r := range line {
		term.PutRune(r)
	}
	term.Resize(8, 24)  // narrow: line wraps into several rows, stays on-screen
	term.Resize(80, 24) // back to wide: line should rejoin unchanged

	got := strings.TrimRight(term.PlainText(), "\n")
	if got != line {
		t.Fatalf("interior spaces collapsed by narrow reflow:\n want %q\n  got %q", line, got)
	}
}
