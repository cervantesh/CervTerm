package accessibility

import "testing"

func benchmarkTerminalInput() TerminalProjectionInput {
	const cols, rows = 80, 24
	cells := make([]TerminalCell, cols*rows)
	for row := 0; row < rows; row++ {
		column := 0
		appendCluster := func(text string, span int) {
			if column+span > cols {
				return
			}
			cells[row*cols+column] = TerminalCell{Text: text, Span: span}
			for continuation := 1; continuation < span; continuation++ {
				cells[row*cols+column+continuation] = TerminalCell{WideContinuation: true}
			}
			column += span
		}
		for _, value := range "CervTerm accessibility benchmark: ASCII " {
			appendCluster(string(value), 1)
		}
		for _, value := range []string{"日", "本", "語"} {
			appendCluster(value, 2)
		}
		appendCluster(" ", 1)
		appendCluster("e\u0301", 1)
		appendCluster(" ", 1)
		appendCluster("👩🏽‍💻", 2)
		appendCluster(" ", 1)
		for _, value := range "مرحبا" {
			appendCluster(string(value), 1)
		}
		for column < cols {
			cells[row*cols+column] = TerminalCell{Blank: true, Span: 1}
			column++
		}
	}
	root := NodeID{Kind: NodeKindWindow, Projection: 1, Object: 1}
	pane := NodeID{Kind: NodeKindPane, Projection: 1, Object: 2, Activation: 1}
	return TerminalProjectionInput{
		ProviderID: 1, Generation: 1, RootID: root, PaneID: pane, RootName: "CervTerm window", PaneName: "Terminal pane",
		Cols: cols, Rows: rows, Cells: cells, Wrapped: make([]bool, rows), CursorVisible: true, Cursor: CellPoint{Row: rows - 1, Col: 10},
		CellWidth: 8, CellHeight: 16, Clip: Rect{Width: cols * 8, Height: rows * 16},
	}
}

func BenchmarkAccessibilitySemanticCapture(b *testing.B) {
	input := benchmarkTerminalInput()
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		input.Generation = uint64(iteration + 1)
		if _, err := ProjectTerminal(input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAccessibilityEventCoalescing(b *testing.B) {
	input := benchmarkTerminalInput()
	previous, err := ProjectTerminal(input)
	if err != nil {
		b.Fatal(err)
	}
	input.Generation++
	input.Cells[(input.Rows-1)*input.Cols].Text = "X"
	next, err := ProjectTerminal(input)
	if err != nil {
		b.Fatal(err)
	}
	input.Generation++
	input.Cells[(input.Rows-2)*input.Cols].Text = "Y"
	middle, err := ProjectTerminal(input)
	if err != nil {
		b.Fatal(err)
	}
	scheduler := NewSemanticScheduler(true)
	defer scheduler.Close()
	intents := IntentDocument | IntentTopology | IntentText | IntentCaret | IntentSelection | IntentFocus
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		scheduler.BeginCycle()
		scheduler.QueueTransition(previous, next, intents)
		scheduler.QueueTransition(next, middle, intents)
		if len(scheduler.Drain()) == 0 {
			b.Fatal("semantic event was not published")
		}
	}
}

func TestAccessibilityBenchmarkAllocationCeilings(t *testing.T) {
	capture := testing.Benchmark(BenchmarkAccessibilitySemanticCapture)
	if capture.AllocsPerOp() > 700 || capture.AllocedBytesPerOp() > 600*1024 {
		t.Fatalf("semantic capture=%d B/op %d allocs/op, ceilings=614400 B/op 700 allocs/op", capture.AllocedBytesPerOp(), capture.AllocsPerOp())
	}
	events := testing.Benchmark(BenchmarkAccessibilityEventCoalescing)
	if events.AllocsPerOp() > 128 || events.AllocedBytesPerOp() > 4*1024 {
		t.Fatalf("event coalescing=%d B/op %d allocs/op, ceilings=4096 B/op 128 allocs/op", events.AllocedBytesPerOp(), events.AllocsPerOp())
	}
}
