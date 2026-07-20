package accessibility

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type terminalGolden struct {
	ProviderID uint64
	Generation uint64
	Focus      NodeID
	Truncated  bool
	Rows       int
	Graphemes  int
	UTF8Bytes  int
	Nodes      []NodeSnapshot
}

func terminalIDs(projection uint64) (NodeID, NodeID) {
	return NodeID{Kind: NodeKindWindow, Projection: projection, Object: 1}, NodeID{Kind: NodeKindPane, Projection: projection, Object: 2, Activation: 1}
}

func projectGolden(t *testing.T, name string, input TerminalProjectionInput) Document {
	t.Helper()
	document, err := ProjectTerminal(input)
	if err != nil {
		t.Fatal(err)
	}
	golden := terminalGolden{
		ProviderID: document.ProviderID(), Generation: document.Generation(), Focus: document.Focus(), Truncated: document.Truncated(),
		Rows: document.RowCount(), Graphemes: document.GraphemeCount(), UTF8Bytes: document.UTF8Bytes(), Nodes: document.Nodes(),
	}
	encoded, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := filepath.Join("testdata", "terminal", name+".golden.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("terminal golden %s changed\n got:\n%s\nwant:\n%s", name, encoded, want)
	}
	return document
}

func TestTerminalProjectionUnicodeWrapCursorSelectionGolden(t *testing.T) {
	root, pane := terminalIDs(1)
	selection := TerminalSelection{Start: CellPoint{Row: 2, Col: 0}, End: CellPoint{Row: 0, Col: 1}}
	input := TerminalProjectionInput{
		ProviderID: 7, Generation: 1, RootID: root, PaneID: pane, RootName: "CervTerm", PaneName: "terminal",
		Cols: 4, Rows: 3,
		Cells: []TerminalCell{
			{Text: "A"}, {Text: "好", Span: 2}, {WideContinuation: true}, {Text: " ", Blank: true},
			{Text: "e\u0301"}, {Text: "B"}, {}, {},
			{Text: "Z"}, {}, {}, {},
		},
		Wrapped:       []bool{true, false, false},
		CursorVisible: true, Cursor: CellPoint{Row: 0, Col: 2}, Selection: &selection,
		OriginX: 10, OriginY: 20, CellWidth: 5, CellHeight: 10, Clip: Rect{X: 10, Y: 20, Width: 20, Height: 30},
	}
	document := projectGolden(t, "unicode-wrap-selection", input)
	node, _ := document.Node(pane)
	if node.Text != "A好e\u0301B\nZ" || node.Caret != 1 || node.Selection != (Span{Start: 1, End: 6}) {
		t.Fatalf("projected terminal node=%#v", node)
	}
}

func TestTerminalProjectionLogicalTextVisualBiDiBoundsGolden(t *testing.T) {
	root, pane := terminalIDs(2)
	input := TerminalProjectionInput{
		ProviderID: 8, Generation: 2, RootID: root, PaneID: pane, RootName: "window", PaneName: "bidi",
		Cols: 3, Rows: 1, Cells: []TerminalCell{{Text: "A"}, {Text: "ב"}, {Text: "C"}}, Wrapped: []bool{false},
		VisualToLogical: [][]int{{2, 1, 0}}, CellWidth: 10, CellHeight: 12, Clip: Rect{Width: 30, Height: 12},
	}
	document := projectGolden(t, "bidi-logical-visual", input)
	node, _ := document.Node(pane)
	if node.Text != "AבC" || node.Rows[0].Bounds[0].X != 20 || node.Rows[0].Bounds[1].X != 10 || node.Rows[0].Bounds[2].X != 0 {
		t.Fatalf("logical/visual mismatch: %#v", node)
	}
}

func TestTerminalProjectionVisibleAlternatePrivacyGolden(t *testing.T) {
	root, pane := terminalIDs(3)
	input := TerminalProjectionInput{
		ProviderID: 9, Generation: 3, RootID: root, PaneID: pane, RootName: "window", PaneName: "alternate",
		Cols: 4, Rows: 1, Cells: []TerminalCell{{Text: "A"}, {Text: "L"}, {Text: "T"}, {}}, Wrapped: []bool{false},
		CursorVisible: true, Cursor: CellPoint{Row: 0, Col: 1}, AlternateScreen: true, DisplayOffset: 5,
		CellWidth: 8, CellHeight: 16, Clip: Rect{Width: 32, Height: 16},
	}
	document := projectGolden(t, "alternate-visible-only", input)
	node, _ := document.Node(pane)
	encoded, _ := json.Marshal(document.Nodes())
	if node.Text != "ALT" || node.HasCaret || strings.Contains(string(encoded), "https://secret.example") || strings.Contains(string(encoded), "history-secret") {
		t.Fatalf("privacy projection node=%#v json=%s", node, encoded)
	}
}

func TestTerminalProjectionClipsWideClusterBounds(t *testing.T) {
	root, pane := terminalIDs(4)
	document, err := ProjectTerminal(TerminalProjectionInput{
		ProviderID: 1, Generation: 1, RootID: root, PaneID: pane,
		Cols: 3, Rows: 1, Cells: []TerminalCell{{Text: "好", Span: 2}, {WideContinuation: true}, {}}, Wrapped: []bool{false},
		OriginX: 10, OriginY: 10, CellWidth: 8, CellHeight: 16, Clip: Rect{X: 14, Y: 12, Width: 8, Height: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	node, _ := document.Node(pane)
	if got := node.Rows[0].Bounds[0]; got != (Rect{X: 14, Y: 12, Width: 8, Height: 8}) {
		t.Fatalf("clipped wide bounds=%#v", got)
	}
}

func TestTerminalProjectionRejectsMalformedGeometryAndMappings(t *testing.T) {
	root, pane := terminalIDs(5)
	base := TerminalProjectionInput{
		ProviderID: 1, Generation: 1, RootID: root, PaneID: pane, Cols: 2, Rows: 1,
		Cells: []TerminalCell{{Text: "a"}, {Text: "b"}}, Wrapped: []bool{false}, CellWidth: 1, CellHeight: 1, Clip: Rect{Width: 2, Height: 1},
	}
	for _, mutate := range []func(*TerminalProjectionInput){
		func(value *TerminalProjectionInput) { value.Cells = value.Cells[:1] },
		func(value *TerminalProjectionInput) { value.Wrapped = nil },
		func(value *TerminalProjectionInput) { value.VisualToLogical = [][]int{{0, 0}} },
		func(value *TerminalProjectionInput) { value.Cells[0].Text = string([]byte{0xff}) },
		func(value *TerminalProjectionInput) { value.Cells[0].Text = "a\nb" },
		func(value *TerminalProjectionInput) { value.Cells[0].Span = 3 },
		func(value *TerminalProjectionInput) {
			value.Cells[0] = TerminalCell{Text: "a", Span: 2}
			value.Cells[1] = TerminalCell{}
		},
		func(value *TerminalProjectionInput) { value.Cells[0] = TerminalCell{WideContinuation: true} },
		func(value *TerminalProjectionInput) { value.Cells[1] = TerminalCell{Text: "secret", Blank: true} },
		func(value *TerminalProjectionInput) { value.Cells[1] = TerminalCell{Text: "secret", TrimBarrier: true} },
		func(value *TerminalProjectionInput) { value.CellWidth = 0 },
	} {
		candidate := base
		candidate.Cells = append([]TerminalCell(nil), base.Cells...)
		mutate(&candidate)
		if _, err := ProjectTerminal(candidate); !errors.Is(err, ErrInvalidProjection) {
			t.Fatalf("malformed projection err=%v candidate=%#v", err, candidate)
		}
	}
}

func TestTerminalProjectionEmojiHangulASCIIGolden(t *testing.T) {
	root, pane := terminalIDs(6)
	input := TerminalProjectionInput{
		ProviderID: 10, Generation: 1, RootID: root, PaneID: pane, RootName: "window", PaneName: "unicode",
		Cols: 7, Rows: 1, Cells: []TerminalCell{{Text: "O"}, {Text: "K"}, {Text: "👩"}, {Text: "\u200d"}, {Text: "💻"}, {Text: "ᄀ"}, {Text: "ᅡ"}},
		Wrapped: []bool{false}, CellWidth: 6, CellHeight: 12, Clip: Rect{Width: 42, Height: 12},
	}
	document := projectGolden(t, "ascii-emoji-hangul", input)
	node, _ := document.Node(pane)
	if node.Text != "OK👩‍💻가" || document.GraphemeCount() != 4 || node.Rows[0].Bounds[2].Width != 18 || node.Rows[0].Bounds[3].Width != 12 {
		t.Fatalf("emoji/Hangul projection document=%#v node=%#v", document, node)
	}
}

func TestTerminalProjectionPrivacyTruncationGolden(t *testing.T) {
	root, pane := terminalIDs(7)
	input := TerminalProjectionInput{
		ProviderID: 11, Generation: 1, RootID: root, PaneID: pane, RootName: "window", PaneName: "terminal",
		Cols: 1, Rows: 1, Cells: []TerminalCell{{Text: strings.Repeat("x", MaxUTF8Bytes+1)}}, Wrapped: []bool{false},
		CellWidth: 8, CellHeight: 16, Clip: Rect{Width: 8, Height: 16},
	}
	document := projectGolden(t, "privacy-truncated", input)
	node, _ := document.Node(pane)
	if !document.Truncated() || node.Text != "" {
		t.Fatalf("privacy truncation document=%#v node=%#v", document, node)
	}
}

func TestTerminalProjectionReflowPreservesLogicalText(t *testing.T) {
	root, pane := terminalIDs(8)
	base := TerminalProjectionInput{ProviderID: 1, Generation: 1, RootID: root, PaneID: pane, CellWidth: 5, CellHeight: 10}
	oneRow := base
	oneRow.Cols, oneRow.Rows = 4, 1
	oneRow.Cells = []TerminalCell{{Text: "A"}, {Text: "B"}, {Text: "C"}, {Text: "D"}}
	oneRow.Wrapped, oneRow.Clip = []bool{false}, Rect{Width: 20, Height: 10}
	twoRows := base
	twoRows.Generation, twoRows.Cols, twoRows.Rows = 2, 2, 2
	twoRows.Cells = []TerminalCell{{Text: "A"}, {Text: "B"}, {Text: "C"}, {Text: "D"}}
	twoRows.Wrapped, twoRows.Clip = []bool{true, false}, Rect{Width: 10, Height: 20}
	first, err := ProjectTerminal(oneRow)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectTerminal(twoRows)
	if err != nil {
		t.Fatal(err)
	}
	firstNode, _ := first.Node(pane)
	secondNode, _ := second.Node(pane)
	if firstNode.Text != "ABCD" || secondNode.Text != firstNode.Text || len(secondNode.Rows) != 2 || !secondNode.Rows[0].SoftWrapped {
		t.Fatalf("reflow first=%#v second=%#v", firstNode, secondNode)
	}
}

func TestTerminalProjectionPaneOriginZoomAndClipAreIndependent(t *testing.T) {
	root, pane := terminalIDs(9)
	base := TerminalProjectionInput{
		ProviderID: 1, Generation: 1, RootID: root, PaneID: pane, Cols: 2, Rows: 1, Cells: []TerminalCell{{Text: "A"}, {Text: "B"}}, Wrapped: []bool{false},
	}
	left := base
	left.OriginX, left.OriginY, left.CellWidth, left.CellHeight, left.Clip = 10, 20, 5, 10, Rect{X: 10, Y: 20, Width: 10, Height: 10}
	right := base
	right.Generation, right.OriginX, right.OriginY, right.CellWidth, right.CellHeight = 2, 100, 30, 9, 18
	right.Clip = Rect{X: 104, Y: 30, Width: 12, Height: 18}
	leftDoc, err := ProjectTerminal(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDoc, err := ProjectTerminal(right)
	if err != nil {
		t.Fatal(err)
	}
	leftNode, _ := leftDoc.Node(pane)
	rightNode, _ := rightDoc.Node(pane)
	if leftNode.Rows[0].Bounds[0] != (Rect{X: 10, Y: 20, Width: 5, Height: 10}) || rightNode.Rows[0].Bounds[0] != (Rect{X: 104, Y: 30, Width: 5, Height: 18}) || rightNode.Rows[0].Bounds[1] != (Rect{X: 109, Y: 30, Width: 7, Height: 18}) {
		t.Fatalf("left=%#v right=%#v", leftNode.Rows[0].Bounds, rightNode.Rows[0].Bounds)
	}
}

func TestTerminalProjectionConcurrentPureCapture(t *testing.T) {
	root, pane := terminalIDs(10)
	input := TerminalProjectionInput{ProviderID: 1, Generation: 1, RootID: root, PaneID: pane, Cols: 2, Rows: 1, Cells: []TerminalCell{{Text: "A"}, {Text: "ב"}}, Wrapped: []bool{false}, VisualToLogical: [][]int{{1, 0}}, CellWidth: 8, CellHeight: 16, Clip: Rect{Width: 16, Height: 16}}
	errorsCh := make(chan error, 16)
	for worker := 0; worker < cap(errorsCh); worker++ {
		go func() {
			for iteration := 0; iteration < 50; iteration++ {
				document, err := ProjectTerminal(input)
				if err == nil && document.GraphemeCount() != 2 {
					err = errors.New("unexpected grapheme count")
				}
				if err != nil {
					errorsCh <- err
					return
				}
			}
			errorsCh <- nil
		}()
	}
	for worker := 0; worker < cap(errorsCh); worker++ {
		if err := <-errorsCh; err != nil {
			t.Fatal(err)
		}
	}
}
