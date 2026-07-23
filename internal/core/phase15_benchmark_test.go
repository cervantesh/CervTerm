package core

import "testing"

var (
	phase15ResizeRows int
	phase15Zones      []SemanticZone
	phase15Truncated  bool
	phase15Terminal   *Terminal
)

func BenchmarkPhase15TerminalStartupMemory(b *testing.B) {
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		phase15Terminal = NewTerminalWithHistory(120, 32, 4096)
	}
}

func BenchmarkPhase15ResizeReflow(b *testing.B) {
	terminal := NewTerminalWithHistory(120, 32, 4096)
	for index := 0; index < 120*256; index++ {
		terminal.PutRune(rune('a' + index%26))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if index&1 == 0 {
			terminal.Resize(80, 40)
		} else {
			terminal.Resize(120, 32)
		}
		phase15ResizeRows = terminal.ScrollbackLines()
	}
}

func BenchmarkPhase15SemanticProjection(b *testing.B) {
	cells := make([]Cell, 120*32)
	for index := range cells {
		switch (index / 40) % 4 {
		case 0:
			cells[index].SemanticKind = SemanticPrompt
		case 1:
			cells[index].SemanticKind = SemanticInput
		case 2:
			cells[index].SemanticKind = SemanticOutput
		}
		if index%40 == 0 && cells[index].SemanticKind != SemanticNone {
			cells[index].SemanticKind |= semanticBoundaryMask
		}
	}
	destination := make([]SemanticZone, 0, 128)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		phase15Zones, phase15Truncated = ProjectSemanticZones(cells, destination)
	}
}
