//go:build glfw

package glfwgl

import (
	"encoding/json"
	"strings"
	"testing"

	"cervterm/internal/accessibility"
	"cervterm/internal/core"
	termmux "cervterm/internal/mux"
	"cervterm/internal/render"
	selectmodel "cervterm/internal/selection"
)

func TestTerminalAccessibilityInputDetachesVisibleValuesAndExcludesMetadata(t *testing.T) {
	root := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: 1, Object: 1}
	pane := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 1, Object: 2, Activation: 1}
	cells := []core.Cell{{Rune: 'L', HyperlinkID: 1}, {Rune: 'I', HyperlinkID: 1}, {Rune: 'N', HyperlinkID: 1}}
	snapshot := render.Snapshot{
		Cols: 3, Rows: 1, HistoryRows: 99, DisplayOffset: 4, CursorRow: 0, CursorCol: 1, CursorVisible: true,
		Title: "title-secret", Cwd: "cwd-secret", Cells: cells, Wrapped: []bool{false},
		Hyperlinks: []core.Hyperlink{{ID: 1, URI: "https://secret.example/token"}},
	}
	selection := selectmodel.Range{Start: selectmodel.Point{Row: 0, Col: 0}, End: selectmodel.Point{Row: 0, Col: 2}}
	input, err := buildTerminalAccessibilityInput(terminalAccessibilityCapture{
		ProviderID: 7, Generation: 1, RootID: root, PaneID: pane, RootName: "window", PaneName: "terminal",
		Snapshot: snapshot, PanePixels: termmux.PixelRect{X: 10, Y: 20, Width: 24, Height: 16}, CellWidth: 8, CellHeight: 16,
		Selection: &selection, Alternate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Cells[0].Rune = 'X'
	snapshot.Wrapped[0] = true
	selection.Start.Col = 2
	document, err := accessibility.ProjectTerminal(input)
	if err != nil {
		t.Fatal(err)
	}
	node, _ := document.Node(pane)
	encoded, _ := json.Marshal(document.Nodes())
	text := string(encoded)
	if node.Text != "LIN" || node.HasCaret || node.Selection != (accessibility.Span{Start: 0, End: 3}) {
		t.Fatalf("detached node=%#v", node)
	}
	for _, secret := range []string{"https://secret.example", "title-secret", "cwd-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("projected private metadata %q: %s", secret, text)
		}
	}
}

func TestTerminalAccessibilityInputPreservesCombiningWideAndBiDiValues(t *testing.T) {
	root := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: 2, Object: 1}
	pane := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 2, Object: 2, Activation: 1}
	combined := core.Cell{Rune: 'e'}
	combined.AppendCombining('\u0301')
	snapshot := render.Snapshot{
		Cols: 4, Rows: 1, CursorVisible: true, CursorCol: 2,
		Cells: []core.Cell{combined, {Rune: '好'}, {WideContinuation: true}, {Rune: 'א'}}, Wrapped: []bool{false},
	}
	input, err := buildTerminalAccessibilityInput(terminalAccessibilityCapture{
		ProviderID: 8, Generation: 1, RootID: root, PaneID: pane, Snapshot: snapshot,
		PanePixels: termmux.PixelRect{Width: 32, Height: 16}, CellWidth: 8, CellHeight: 16, Bidi: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Cells[0].Text != "e\u0301" || input.Cells[1].Span != 2 || !input.Cells[2].WideContinuation || len(input.VisualToLogical) != 1 {
		t.Fatalf("input=%#v", input)
	}
	snapshot.Cells[0].AppendCombining('\u0327')
	if input.Cells[0].Text != "e\u0301" {
		t.Fatalf("source combining mutation reached detached input: %q", input.Cells[0].Text)
	}
	document, err := accessibility.ProjectTerminal(input)
	if err != nil {
		t.Fatal(err)
	}
	node, _ := document.Node(pane)
	if node.Text != "e\u0301好א" || !node.HasCaret || node.Caret != 1 {
		t.Fatalf("node=%#v", node)
	}
}

func TestTerminalAccessibilityInputRejectsNonDetachedSnapshotShape(t *testing.T) {
	root := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: 3, Object: 1}
	pane := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 3, Object: 2, Activation: 1}
	_, err := buildTerminalAccessibilityInput(terminalAccessibilityCapture{
		ProviderID: 1, Generation: 1, RootID: root, PaneID: pane,
		Snapshot: render.Snapshot{Cols: 2, Rows: 1, Cells: []core.Cell{{Rune: 'x'}}, Wrapped: []bool{false}},
	})
	if err != accessibility.ErrInvalidProjection {
		t.Fatalf("shape err=%v", err)
	}
}

func TestTerminalAccessibilityInputMirrorsMetadataAwareTrailingSpaceTextWithoutLeakingMetadata(t *testing.T) {
	root := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: 4, Object: 1}
	pane := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 4, Object: 2, Activation: 1}
	snapshot := render.Snapshot{
		Cols: 3, Rows: 1, Cells: []core.Cell{{Rune: 'A'}, {Rune: ' '}, {HyperlinkID: 1}}, Wrapped: []bool{false},
		Hyperlinks: []core.Hyperlink{{ID: 1, URI: "https://secret.example"}},
	}
	input, err := buildTerminalAccessibilityInput(terminalAccessibilityCapture{
		ProviderID: 1, Generation: 1, RootID: root, PaneID: pane, Snapshot: snapshot,
		PanePixels: termmux.PixelRect{Width: 24, Height: 16}, CellWidth: 8, CellHeight: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	document, err := accessibility.ProjectTerminal(input)
	if err != nil {
		t.Fatal(err)
	}
	node, _ := document.Node(pane)
	encoded, _ := json.Marshal(node)
	if node.Text != "A " || strings.Contains(string(encoded), "secret.example") {
		t.Fatalf("metadata-aware row=%q json=%s", node.Text, encoded)
	}
}
