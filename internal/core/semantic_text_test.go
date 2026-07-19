package core

import "testing"

func TestSemanticRangeTextExtractsOnlyTaggedCells(t *testing.T) {
	term := NewTerminal(8, 2)
	term.SetSemanticKind(SemanticInput)
	term.PutRune('e')
	term.PutRune('\u0301')
	term.PutRune('好')
	term.SetSemanticKind(SemanticOutput)
	term.PutRune('x')
	ranges, _ := term.SemanticHistory()
	if len(ranges) != 2 {
		t.Fatalf("ranges=%#v", ranges)
	}
	text, ok := term.SemanticRangeText(ranges[0])
	if !ok || text != "é好" {
		t.Fatalf("text=%q ok=%v", text, ok)
	}
}

func TestSemanticRangeTextPreservesBlankSemanticRows(t *testing.T) {
	term := NewTerminal(4, 3)
	term.SetSemanticKind(SemanticOutput)
	term.PutRune('A')
	term.CarriageReturn()
	term.NewLine()
	term.NewLine()
	term.PutRune('B')
	ranges, _ := term.SemanticHistory()
	if len(ranges) != 1 {
		t.Fatalf("ranges=%#v", ranges)
	}
	text, ok := term.SemanticRangeText(ranges[0])
	if !ok || text != "A\n\nB" {
		t.Fatalf("text=%q ok=%v", text, ok)
	}
}

func TestSemanticRangeTextRejectsForgedRange(t *testing.T) {
	term := NewTerminal(4, 1)
	if _, ok := term.SemanticRangeText(SemanticRange{Kind: SemanticOutput, Start: SemanticPoint{-1, 0}, End: SemanticPoint{0, 1}}); ok {
		t.Fatal("forged range accepted")
	}
}

func TestSemanticRangeTextJoinsSoftWrappedRows(t *testing.T) {
	term := NewTerminal(4, 2)
	term.SetSemanticKind(SemanticOutput)
	for _, r := range "abcdef" {
		term.PutRune(r)
	}
	ranges, _ := term.SemanticHistory()
	text, ok := term.SemanticRangeText(ranges[0])
	if !ok || text != "abcdef" {
		t.Fatalf("text=%q ok=%v ranges=%#v", text, ok, ranges)
	}
	term.Resize(3, 3)
	ranges, _ = term.SemanticHistory()
	text, ok = term.SemanticRangeText(ranges[0])
	if !ok || text != "abcdef" {
		t.Fatalf("reflow text=%q ok=%v", text, ok)
	}
}
