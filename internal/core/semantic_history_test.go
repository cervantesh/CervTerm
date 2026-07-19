package core

import "testing"

func TestSemanticHistoryOrdersAndMergesPhysicalRows(t *testing.T) {
	term := NewTerminal(4, 3)
	term.SetSemanticKind(SemanticPrompt)
	term.PutRune('P')
	term.SetSemanticKind(SemanticInput)
	term.PutRune('I')
	term.CarriageReturn()
	term.NewLine()
	term.SetSemanticKind(SemanticOutput)
	term.NewLine()
	term.PutRune('O')
	ranges, truncated := term.SemanticHistory()
	if truncated || len(ranges) != 3 {
		t.Fatalf("ranges=%#v truncated=%v", ranges, truncated)
	}
	if ranges[0].Kind != SemanticPrompt || ranges[1].Kind != SemanticInput || ranges[2].Kind != SemanticOutput {
		t.Fatalf("ranges=%#v", ranges)
	}
	if ranges[2].Start.GlobalRow != 1 || ranges[2].End.GlobalRow != 2 {
		t.Fatalf("blank output row did not merge: %#v", ranges[2])
	}
}

func TestSemanticHistoryKeepsNewestRangesWhenBounded(t *testing.T) {
	term := NewTerminal(MaxSemanticHistoryRanges*2+1, 1)
	for index := range term.cells {
		if index%2 == 0 {
			term.cells[index].SemanticKind = SemanticPrompt
		}
	}
	ranges, truncated := term.SemanticHistory()
	if !truncated || len(ranges) != MaxSemanticHistoryRanges {
		t.Fatalf("ranges=%d truncated=%v", len(ranges), truncated)
	}
	if ranges[len(ranges)-1].End.Col != len(term.cells) {
		t.Fatalf("newest range=%#v", ranges[len(ranges)-1])
	}
}

func TestSemanticHistoryCellBudgetReportsOldestTruncation(t *testing.T) {
	const cols = 256
	term := NewTerminalWithHistory(cols, 2, 10000)
	row := make([]Cell, cols)
	row[0] = Cell{SemanticKind: SemanticPrompt | semanticBoundaryMask}
	for index := 0; index < MaxSemanticHistoryCells/cols+10; index++ {
		term.appendScrollbackLine(row, false)
	}
	ranges, truncated := term.SemanticHistory()
	if !truncated || len(ranges) == 0 {
		t.Fatalf("ranges=%d truncated=%v", len(ranges), truncated)
	}
	oldestAllowed := term.ScrollbackLines() + term.Rows() - max(1, MaxSemanticHistoryCells/cols)
	if ranges[0].Start.GlobalRow < oldestAllowed {
		t.Fatalf("oldest range=%#v allowed=%d", ranges[0], oldestAllowed)
	}
}

func TestSemanticHistoryPreservesSameKindMarkerBoundaries(t *testing.T) {
	term := NewTerminal(4, 1)
	term.SetSemanticKind(SemanticPrompt)
	term.PutRune('A')
	term.SetSemanticKind(SemanticPrompt)
	term.PutRune('B')
	ranges, truncated := term.SemanticHistory()
	if truncated || len(ranges) != 2 || !ranges[0].StartsAtMarker || !ranges[1].StartsAtMarker {
		t.Fatalf("ranges=%#v truncated=%v", ranges, truncated)
	}
	zones, _ := ProjectSemanticZones(term.cells, nil)
	if len(zones) != 2 {
		t.Fatalf("zones=%#v", zones)
	}
}

func TestSemanticHistoryRejectsSingleRowBeyondCellBudget(t *testing.T) {
	term := NewTerminal(MaxSemanticHistoryCells+1, 1)
	term.cells[len(term.cells)-1].SemanticKind = SemanticPrompt
	ranges, truncated := term.SemanticHistory()
	if !truncated || len(ranges) != 0 {
		t.Fatalf("ranges=%d truncated=%v", len(ranges), truncated)
	}
}

func TestScrollViewportToGlobalRowUsesCanonicalPhysicalCoordinates(t *testing.T) {
	term := NewTerminalWithHistory(4, 2, 8)
	for i := 0; i < 6; i++ {
		term.appendScrollbackLine(make([]Cell, 4), false)
	}
	if !term.ScrollViewportToGlobalRow(2) || term.ViewportTopGlobalRow() != 2 {
		t.Fatalf("top=%d offset=%d", term.ViewportTopGlobalRow(), term.DisplayOffset())
	}
	if !term.ScrollViewportToGlobalRow(99) || term.DisplayOffset() != 0 {
		t.Fatalf("bottom offset=%d", term.DisplayOffset())
	}
	if !term.ScrollViewportToGlobalRow(-1) || term.ViewportTopGlobalRow() != 0 {
		t.Fatalf("oldest top=%d", term.ViewportTopGlobalRow())
	}
}
