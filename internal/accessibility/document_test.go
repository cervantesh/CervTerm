package accessibility

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
)

func testIDs() (NodeID, NodeID) {
	root := NodeID{Kind: NodeKindWindow, Projection: 1, Object: 1}
	pane := NodeID{Kind: NodeKindPane, Projection: 1, Object: 2, Activation: 3}
	return root, pane
}

func testDocument(t *testing.T, generation uint64) Document {
	t.Helper()
	root, pane := testIDs()
	caret := 4
	selection := Span{Start: 1, End: 4}
	document, err := NewDocument(DocumentDraft{
		ProviderID: 7,
		Generation: generation,
		Focus:      pane,
		Nodes: []NodeDraft{
			{ID: root, Role: RoleWindow, Name: "CervTerm"},
			{ID: pane, Parent: root, Role: RoleTerminal, Name: "terminal", Caret: &caret, Selection: &selection, Rows: []RowDraft{
				{Text: "A好", Bounds: []Rect{{X: 1, Y: 2, Width: 3, Height: 4}, {X: 4, Y: 2, Width: 6, Height: 4}}},
				{Text: "e\u0301", Bounds: []Rect{{X: 1, Y: 6, Width: 3, Height: 4}}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestDocumentBuildsDetachedUnicodeSnapshot(t *testing.T) {
	document := testDocument(t, 1)
	_, pane := testIDs()
	if document.ProviderID() != 7 || document.Generation() != 1 || document.Focus() != pane || document.Truncated() || document.RowCount() != 2 || document.GraphemeCount() != 4 {
		t.Fatalf("document metadata provider=%d generation=%d focus=%#v truncated=%v rows=%d graphemes=%d", document.ProviderID(), document.Generation(), document.Focus(), document.Truncated(), document.RowCount(), document.GraphemeCount())
	}
	node, ok := document.Node(pane)
	if !ok || node.Text != "A好\ne\u0301" || !node.HasCaret || node.Caret != 4 || !node.HasSelect || node.Selection != (Span{Start: 1, End: 4}) {
		t.Fatalf("node=%#v ok=%v", node, ok)
	}
	if len(node.Rows) != 2 || node.Rows[0].StartGrapheme != 0 || node.Rows[0].EndGrapheme != 2 || node.Rows[1].StartGrapheme != 3 || node.Rows[1].EndGrapheme != 4 {
		t.Fatalf("rows=%#v", node.Rows)
	}
	node.Rows[0].Bounds[0].X = 999
	node.Rows[0].Text = "mutated"
	again, _ := document.Node(pane)
	if again.Rows[0].Bounds[0].X != 1 || again.Rows[0].Text != "A好" {
		t.Fatalf("caller mutated document: %#v", again.Rows[0])
	}
}

func TestDocumentRejectsInvalidIdentityTextBoundsAndRanges(t *testing.T) {
	root, pane := testIDs()
	valid := DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow}, {ID: pane, Parent: root, Role: RoleTerminal}}}
	for _, test := range []struct {
		name  string
		draft DocumentDraft
		want  error
	}{
		{name: "zero provider", draft: DocumentDraft{Generation: 1, Nodes: valid.Nodes}, want: ErrInvalidDocument},
		{name: "max generation", draft: DocumentDraft{ProviderID: 1, Generation: ^uint64(0), Nodes: valid.Nodes}, want: ErrInvalidDocument},
		{name: "bad root", draft: DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: pane, Parent: root, Role: RoleTerminal}}}, want: ErrInvalidIdentity},
		{name: "duplicate", draft: DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow}, {ID: root, Parent: root, Role: RoleTab}}}, want: ErrInvalidIdentity},
		{name: "invalid utf8", draft: DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow, Rows: []RowDraft{{Text: string([]byte{0xff})}}}}}, want: ErrInvalidText},
		{name: "embedded newline", draft: DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow, Rows: []RowDraft{{Text: "a\nb"}}}}}, want: ErrInvalidText},
		{name: "cross projection", draft: DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow}, {ID: NodeID{Kind: NodeKindPane, Projection: 2, Object: 2, Activation: 1}, Parent: root, Role: RoleTerminal}}}, want: ErrInvalidIdentity},
		{name: "missing activation", draft: DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow}, {ID: NodeID{Kind: NodeKindPane, Projection: 1, Object: 2}, Parent: root, Role: RoleTerminal}}}, want: ErrInvalidIdentity},
		{name: "bounds count", draft: DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow, Rows: []RowDraft{{Text: "ab", Bounds: []Rect{{}}}}}}}, want: ErrInvalidBounds},
		{name: "nonfinite bounds", draft: DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow, Rows: []RowDraft{{Text: "a", Bounds: []Rect{{X: math.NaN()}}}}}}}, want: ErrInvalidBounds},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewDocument(test.draft)
			if !errors.Is(err, test.want) {
				t.Fatalf("err=%v want=%v", err, test.want)
			}
		})
	}

	negative := -1
	valid.Nodes[1].Caret = &negative
	if _, err := NewDocument(valid); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("negative caret err=%v", err)
	}

	missing := root
	missing.Object = 99
	long := strings.Repeat("x", MaxGraphemes+1)
	if _, err := NewDocument(DocumentDraft{ProviderID: 1, Generation: 1, Focus: missing, Nodes: []NodeDraft{{ID: root, Role: RoleWindow, Rows: []RowDraft{{Text: long}}}}}); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("content truncation suppressed invalid focus: %v", err)
	}
}

func TestDocumentAppliesEveryGlobalBoundDeterministically(t *testing.T) {
	root, _ := testIDs()

	rows := make([]RowDraft, MaxRows+1)
	document, err := NewDocument(DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow, Rows: rows}}})
	if err != nil || !document.Truncated() || document.RowCount() != MaxRows {
		t.Fatalf("row bound document=%#v err=%v", document, err)
	}

	text := strings.Repeat("x", MaxGraphemes+1)
	document, err = NewDocument(DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow, Rows: []RowDraft{{Text: text}}}}})
	if err != nil || !document.Truncated() || document.GraphemeCount() != MaxGraphemes {
		t.Fatalf("grapheme bound count=%d truncated=%v err=%v", document.GraphemeCount(), document.Truncated(), err)
	}

	largeCluster := "a" + strings.Repeat("\u0301", 100)
	text = strings.Repeat(largeCluster+"b", 11_000)
	document, err = NewDocument(DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow, Rows: []RowDraft{{Text: text}}}}})
	if err != nil || !document.Truncated() || document.UTF8Bytes() > MaxUTF8Bytes || document.GraphemeCount() >= MaxGraphemes {
		t.Fatalf("byte bound bytes=%d graphemes=%d truncated=%v err=%v", document.UTF8Bytes(), document.GraphemeCount(), document.Truncated(), err)
	}

	nodes := make([]NodeDraft, MaxNodes+1)
	nodes[0] = NodeDraft{ID: root, Role: RoleWindow}
	for index := 1; index < len(nodes); index++ {
		nodes[index] = NodeDraft{ID: NodeID{Kind: NodeKindItem, Projection: 1, Object: uint64(index + 1), Activation: 1}, Parent: root, Role: RoleListItem}
	}
	document, err = NewDocument(DocumentDraft{ProviderID: 1, Generation: 1, Nodes: nodes})
	if err != nil || !document.Truncated() || document.NodeCount() != MaxNodes {
		t.Fatalf("node bound count=%d truncated=%v err=%v", document.NodeCount(), document.Truncated(), err)
	}
}

func TestDocumentUsesUAX29GraphemeBoundaries(t *testing.T) {
	root, _ := testIDs()
	text := "가" // decomposed Hangul L+V is one extended grapheme cluster.
	document, err := NewDocument(DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{ID: root, Role: RoleWindow, Rows: []RowDraft{{Text: text, Bounds: []Rect{{Width: 1, Height: 1}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if document.GraphemeCount() != 1 {
		t.Fatalf("decomposed Hangul graphemes=%d", document.GraphemeCount())
	}
}

func TestDocumentClampsCaretAndSelectionWhenContentTruncates(t *testing.T) {
	root, _ := testIDs()
	caret := MaxGraphemes + 50
	selection := Span{Start: MaxGraphemes - 1, End: MaxGraphemes + 100}
	document, err := NewDocument(DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{
		ID: root, Role: RoleWindow, Caret: &caret, Selection: &selection, Rows: []RowDraft{{Text: strings.Repeat("x", MaxGraphemes+100)}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	node, _ := document.Node(root)
	if !document.Truncated() || node.Caret != MaxGraphemes || node.Selection != (Span{Start: MaxGraphemes - 1, End: MaxGraphemes}) {
		t.Fatalf("document truncated=%v node=%#v", document.Truncated(), node)
	}
}

func TestDocumentConcurrentReadsRemainDetached(t *testing.T) {
	document := testDocument(t, 1)
	_, pane := testIDs()
	var wait sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 100; iteration++ {
				node, ok := document.Node(pane)
				if !ok || node.Text == "" || len(document.Nodes()) != 2 {
					t.Errorf("detached read failed")
					return
				}
				node.Rows[0].Bounds[0].X++
			}
		}()
	}
	wait.Wait()
}

func TestDocumentSoftWrappedRowsDoNotInsertLogicalNewline(t *testing.T) {
	root, _ := testIDs()
	document, err := NewDocument(DocumentDraft{ProviderID: 1, Generation: 1, Nodes: []NodeDraft{{
		ID: root, Role: RoleWindow, Rows: []RowDraft{{Text: "ab", SoftWrapped: true}, {Text: "cd"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	node, _ := document.Node(root)
	if node.Text != "abcd" || document.GraphemeCount() != 4 || !node.Rows[0].SoftWrapped || node.Rows[1].StartGrapheme != 2 {
		t.Fatalf("soft-wrapped node=%#v graphemes=%d", node, document.GraphemeCount())
	}
}
