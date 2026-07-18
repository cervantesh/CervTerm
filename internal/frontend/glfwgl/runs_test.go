package glfwgl

import (
	"testing"

	"cervterm/internal/core"
)

func symbolCells(s string) []core.Cell {
	cells := make([]core.Cell, 0, len(s))
	for _, r := range s {
		cells = append(cells, core.Cell{Rune: r})
	}
	return cells
}

func TestDetectLigatureRunAlphabet(t *testing.T) {
	// A run of alphabet symbols is detected end to end.
	cells := symbolCells("->=")
	run, ok := detectLigatureRun(cells, 0, -1)
	if !ok {
		t.Fatalf("expected a ligature run for symbol cells")
	}
	if run.Text != "->=" || run.CellSpan != 3 {
		t.Fatalf("run = %#v", run)
	}
	for _, text := range []string{"->", "=>", "!=", "==="} {
		run, ok := detectLigatureRun(symbolCells(text), 0, -1)
		if !ok || run.Text != text || run.CellSpan != len(text) {
			t.Fatalf("grid-safe run %q = %#v ok=%v", text, run, ok)
		}
	}

	// Letters and digits are excluded, so the run stops at the first non-symbol.
	cells = symbolCells("->a>")
	run, ok = detectLigatureRun(cells, 0, -1)
	if !ok || run.Text != "->" || run.CellSpan != 2 {
		t.Fatalf("letters must break the run; got %#v ok=%v", run, ok)
	}

	// A single symbol is below the minimum length and falls through per-cell.
	if _, ok := detectLigatureRun(symbolCells("-a"), 0, -1); ok {
		t.Fatalf("single symbol must not form a run")
	}
}

func TestDetectLigatureRunAttrEqualityInverse(t *testing.T) {
	// A differing Inverse flag between cells splits the run: inverse swaps FG/BG
	// per cell, so the span cannot share one uniform color.
	cells := symbolCells("->=")
	cells[1].Attr.Inverse = true
	if _, ok := detectLigatureRun(cells, 0, -1); ok {
		t.Fatalf("inverse-mixed run must not ligate")
	}
	// Cells sharing identical (inverse) attrs still form a run.
	cells = symbolCells("->=")
	cells[1].Attr.Inverse = true
	cells[2].Attr.Inverse = true
	run, ok := detectLigatureRun(cells, 1, -1)
	if !ok || run.Text != ">=" || run.CellSpan != 2 {
		t.Fatalf("uniform-inverse run = %#v ok=%v", run, ok)
	}
}

func TestDetectLigatureRunCursorSplit(t *testing.T) {
	cells := symbolCells("->=")
	if _, ok := detectLigatureRun(cells, 0, 1); ok {
		t.Fatalf("cursor inside the run must reject it")
	}
	if _, ok := detectLigatureRun(cells, 0, 0); ok {
		t.Fatalf("cursor on the run start must reject it")
	}
	// Cursor elsewhere on the row leaves the run intact.
	if _, ok := detectLigatureRun(cells, 0, 7); !ok {
		t.Fatalf("cursor off the run must allow it")
	}
}

func TestDetectLigatureRunLengthBounds(t *testing.T) {
	// Nine symbols: the run is capped at the 8-cell maximum.
	cells := symbolCells("+++++++++")
	run, ok := detectLigatureRun(cells, 0, -1)
	if !ok {
		t.Fatalf("expected a capped run")
	}
	if run.CellSpan != ligatureMaxRunCells || len([]rune(run.Text)) != ligatureMaxRunCells {
		t.Fatalf("run must cap at %d cells, got %#v", ligatureMaxRunCells, run)
	}
}

func TestDetectLigatureRunSkipsWideAndCombining(t *testing.T) {
	cells := symbolCells("->")
	cells[1].AppendCombining('́')
	if _, ok := detectLigatureRun(cells, 0, -1); ok {
		t.Fatalf("combining marks must break the run")
	}
	cells = symbolCells("->")
	cells[1].WideContinuation = true
	if _, ok := detectLigatureRun(cells, 0, -1); ok {
		t.Fatalf("continuation cells must break the run")
	}
}
