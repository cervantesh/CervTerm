package core

import (
	"testing"
	"unsafe"
)

func TestSemanticKindPreservesCompactCellAndPrintedCluster(t *testing.T) {
	if got := unsafe.Sizeof(Cell{}); got != 32 {
		t.Fatalf("Cell size=%d", got)
	}
	term := NewTerminal(6, 2)
	term.SetSemanticKind(SemanticInput)
	term.PutRune(' ')
	term.PutRune('好')
	term.PutRune('\u0301')
	cells := make([]Cell, 12)
	term.CopyView(cells)
	for _, index := range []int{0, 1, 2} {
		if SemanticCellKind(cells[index]) != SemanticInput {
			t.Fatalf("cell %d kind=%v", index, SemanticCellKind(cells[index]))
		}
	}
	term.SetCursor(0, 0)
	term.EraseChars(3)
	term.CopyView(cells)
	for _, cell := range cells[:3] {
		if SemanticCellKind(cell) != SemanticNone {
			t.Fatal("erase retained semantic kind")
		}
	}
}

func TestSemanticKindSurvivesScrollbackReflowAndAlternateIsolation(t *testing.T) {
	term := NewTerminalWithHistory(4, 2, 4)
	term.SetSemanticKind(SemanticPrompt)
	term.PutRune('P')
	for i := 0; i < 3; i++ {
		term.CarriageReturn()
		term.NewLine()
		term.PutRune(rune('0' + i))
	}
	term.Resize(3, 3)
	rows, _ := term.physicalRows()
	found := false
	for _, row := range rows {
		for _, cell := range row {
			if cell.Rune == 'P' && SemanticCellKind(cell) == SemanticPrompt {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("scrollback/reflow lost semantic kind")
	}
	term.SetSemanticKind(SemanticInput)
	term.SetAlternateScreenMode(true)
	if term.SemanticKind() != SemanticNone {
		t.Fatal("primary semantic state leaked to alternate")
	}
	term.SetSemanticKind(SemanticOutput)
	term.PutRune('A')
	term.SetAlternateScreenMode(false)
	if term.SemanticKind() != SemanticInput {
		t.Fatalf("restored kind=%v", term.SemanticKind())
	}
	term.Reset()
	if term.SemanticKind() != SemanticNone {
		t.Fatal("reset retained semantic state")
	}
}

func TestProjectSemanticZonesBoundedAndDetached(t *testing.T) {
	cells := []Cell{{SemanticKind: SemanticPrompt}, {SemanticKind: SemanticPrompt}, {}, {SemanticKind: SemanticInput}, {SemanticKind: SemanticOutput}}
	zones, truncated := ProjectSemanticZones(cells, nil)
	if truncated || len(zones) != 3 || zones[0] != (SemanticZone{Kind: SemanticPrompt, Start: 0, End: 2}) || zones[1] != (SemanticZone{Kind: SemanticInput, Start: 3, End: 4}) {
		t.Fatalf("zones=%#v truncated=%v", zones, truncated)
	}
	many := make([]Cell, MaxSemanticZones*2+1)
	for i := range many {
		if i%2 == 0 {
			many[i].SemanticKind = SemanticPrompt
		}
	}
	zones, truncated = ProjectSemanticZones(many, zones)
	if !truncated || len(zones) != MaxSemanticZones {
		t.Fatalf("zones=%d truncated=%v", len(zones), truncated)
	}
}
