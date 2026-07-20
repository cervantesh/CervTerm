package quickselect

import (
	"strings"
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/mux"
)

func snapshotText(rows ...string) mux.QuickSelectSnapshot {
	cols := 1
	for _, row := range rows {
		if len([]rune(row)) > cols {
			cols = len([]rune(row))
		}
	}
	cells := make([]core.Cell, len(rows)*cols)
	for r, row := range rows {
		for c, ch := range []rune(row) {
			cells[r*cols+c].Rune = ch
		}
	}
	return mux.QuickSelectSnapshot{PaneID: 1, FocusedPaneID: 1, Cols: cols, Rows: len(rows), Cells: cells, Wrapped: make([]bool, len(rows))}
}

func TestFindHTTPAndPreparedRulesDeterministically(t *testing.T) {
	s := snapshotText("see https://example.com/path, and ticket ABC-42")
	rule, err := PrepareRule("ticket", `ABC-[0-9]+`, 5)
	if err != nil {
		t.Fatal(err)
	}
	got := Find(s, []PreparedRule{rule})
	if len(got) != 2 {
		t.Fatalf("candidates=%#v", got)
	}
	if got[0].Text != "https://example.com/path" || got[0].RuleID != "builtin:http" {
		t.Fatalf("http=%#v", got[0])
	}
	if got[1].Text != "ABC-42" || got[1].RuleID != "ticket" {
		t.Fatalf("rule=%#v", got[1])
	}
	if got[0].Label == got[1].Label || got[0].Label == "" {
		t.Fatalf("labels=%q,%q", got[0].Label, got[1].Label)
	}
}

func TestFindExpandsWideAndCombiningCells(t *testing.T) {
	cells := make([]core.Cell, 5)
	cells[0] = core.NewCellWithCombining('e', core.Attr{}, '\u0301')
	cells[1].Rune = '界'
	cells[2].WideContinuation = true
	cells[3].Rune = 'x'
	cells[4].Rune = ' '
	s := mux.QuickSelectSnapshot{PaneID: 1, FocusedPaneID: 1, Cols: 5, Rows: 1, Cells: cells, Wrapped: []bool{false}}
	rule, err := PrepareRule("cluster", `é界`, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := Find(s, []PreparedRule{rule})
	if len(got) != 1 {
		t.Fatalf("got=%#v", got)
	}
	if got[0].Start.Cell != 0 || got[0].End.Cell != 3 || got[0].Text != "é界" {
		t.Fatalf("candidate=%#v", got[0])
	}
}

func TestFindWrappedRowsAndPriorityOverlap(t *testing.T) {
	s := snapshotText("https://exa", "mple.test")
	s.Wrapped[0] = true
	rule, err := PrepareRule("preferred", `https://example\.test`, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := Find(s, []PreparedRule{rule})
	if len(got) != 1 || got[0].RuleID != "preferred" {
		t.Fatalf("got=%#v", got)
	}
	if got[0].Start.GlobalRow != 0 || got[0].End.GlobalRow != 1 {
		t.Fatalf("span=%#v", got[0])
	}
}

func TestCandidateAndLabelBounds(t *testing.T) {
	s := snapshotText(strings.Repeat("x ", MaxCandidates+20))
	rule, err := PrepareRule("x", `x`, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := Find(s, []PreparedRule{rule})
	if len(got) != MaxCandidates {
		t.Fatalf("count=%d", len(got))
	}
	labels := Labels(30)
	for i, a := range labels {
		for j, b := range labels {
			if i != j && strings.HasPrefix(a, b) {
				t.Fatalf("labels not prefix-free: %q %q", a, b)
			}
		}
	}
}

func TestPrepareRuleRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct{ id, pattern string }{{"", "x"}, {"x", ""}, {"x", "["}, {"x", strings.Repeat("x", MaxPattern+1)}} {
		if _, err := PrepareRule(tc.id, tc.pattern, 0); err == nil {
			t.Fatalf("accepted %#v", tc)
		}
	}
}
