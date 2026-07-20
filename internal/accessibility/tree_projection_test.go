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

func treeID(kind NodeKind, projection, object, activation uint64) NodeID {
	return NodeID{Kind: kind, Projection: projection, Object: object, Activation: activation}
}

func treeTerminal(provider, generation uint64, window, pane NodeID, name, text string) TerminalProjectionInput {
	cells := make([]TerminalCell, 0, len([]rune(text)))
	for _, value := range text {
		cells = append(cells, TerminalCell{Text: string(value)})
	}
	return TerminalProjectionInput{
		ProviderID: provider, Generation: generation, RootID: window, PaneID: pane, PaneName: name,
		Cols: len(cells), Rows: 1, Cells: cells, Wrapped: []bool{false}, CursorVisible: true,
		CellWidth: 8, CellHeight: 16, Clip: Rect{Width: float64(len(cells) * 8), Height: 16},
	}
}

func composeTreeGolden(t *testing.T, name string, input TreeProjectionInput) Document {
	t.Helper()
	document, ok, err := ComposeTree(input)
	if err != nil || !ok {
		t.Fatalf("compose ok=%v err=%v", ok, err)
	}
	value := terminalGolden{
		ProviderID: document.ProviderID(), Generation: document.Generation(), Focus: document.Focus(), Truncated: document.Truncated(),
		Rows: document.RowCount(), Graphemes: document.GraphemeCount(), UTF8Bytes: document.UTF8Bytes(), Nodes: document.Nodes(),
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := filepath.Join("testdata", "tree", name+".golden.json")
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
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(encoded, want) {
		t.Fatalf("golden mismatch %s\n%s", path, encoded)
	}
	return document
}

func TestComposeTreeActiveTabFocusAndPrivacyGolden(t *testing.T) {
	window := treeID(NodeKindWindow, 20, 1, 0)
	active := treeID(NodeKindTab, 20, 2, 0)
	hidden := treeID(NodeKindTab, 20, 3, 0)
	left := treeID(NodeKindPane, 20, 4, 1)
	right := treeID(NodeKindPane, 20, 5, 2)
	input := TreeProjectionInput{
		ProviderID: 3, Generation: 1, Visible: true, Window: TreeWindow{ID: window, Name: "safe window", Focused: true, Tabs: []TreeTab{
			{ID: hidden, Name: "hidden-title-secret", Panes: []TreePane{{Terminal: treeTerminal(3, 1, window, treeID(NodeKindPane, 20, 9, 1), "hidden", "hidden-text-secret")}}},
			{ID: active, Name: "active", Active: true, Panes: []TreePane{{Terminal: treeTerminal(3, 1, window, left, "left", "LEFT")}, {Terminal: treeTerminal(3, 1, window, right, "right", "RIGHT")}}},
		}}, FocusedPane: right,
	}
	document := composeTreeGolden(t, "active-tab-privacy", input)
	encoded, _ := json.Marshal(document.Nodes())
	if document.Focus() != right || strings.Contains(string(encoded), "secret") || len(document.Nodes()) != 4 {
		t.Fatalf("document focus=%#v nodes=%s", document.Focus(), encoded)
	}
}

func TestComposeTreeModalSearchPreeditPrecedenceAndStableActivation(t *testing.T) {
	window := treeID(NodeKindWindow, 21, 1, 0)
	tab := treeID(NodeKindTab, 21, 2, 0)
	pane := treeID(NodeKindPane, 21, 3, 4)
	modalID := treeID(NodeKindInput, 21, 10, 7)
	searchID := treeID(NodeKindInput, 21, 11, 8)
	preeditID := treeID(NodeKindInput, 21, 12, 9)
	caret, selection := 1, Span{Start: 0, End: 1}
	base := TreeProjectionInput{
		ProviderID: 4, Generation: 1, Visible: true, FocusedPane: pane,
		Window:  TreeWindow{ID: window, Focused: true, Tabs: []TreeTab{{ID: tab, Active: true, Panes: []TreePane{{Terminal: treeTerminal(4, 1, window, pane, "terminal", "shell")}}}}},
		Modal:   &TreeInput{ID: modalID, Parent: window, Role: RoleDialog, Name: "palette", Text: "m", Caret: &caret},
		Search:  &TreeInput{ID: searchID, Parent: pane, Role: RoleTextField, Name: "search", Text: "search-secret"},
		Preedit: &TreeInput{ID: preeditID, Parent: pane, Role: RoleTextField, Name: "preedit", Text: "ime-secret", Selection: &selection},
	}
	document := composeTreeGolden(t, "modal-precedence", base)
	encoded, _ := json.Marshal(document.Nodes())
	if document.Focus() != modalID || strings.Contains(string(encoded), "search-secret") || strings.Contains(string(encoded), "ime-secret") {
		t.Fatalf("modal precedence focus=%#v nodes=%s", document.Focus(), encoded)
	}
	base.Generation++
	base.Window.Tabs[0].Panes[0].Terminal.Generation++
	base.Modal.Text = "mutated"
	updated, ok, err := ComposeTree(base)
	if err != nil || !ok || updated.Focus() != modalID {
		t.Fatalf("stable activation ok=%v err=%v focus=%#v", ok, err, updated.Focus())
	}
	base.Modal = nil
	searched, ok, err := ComposeTree(base)
	if err != nil || !ok || searched.Focus() != searchID {
		t.Fatalf("search precedence ok=%v err=%v focus=%#v", ok, err, searched.Focus())
	}
	base.Search = nil
	preedit, ok, err := ComposeTree(base)
	preeditJSON, _ := json.Marshal(preedit.Nodes())
	if err != nil || !ok || preedit.Focus() != pane || !strings.Contains(string(preeditJSON), "ime-secret") {
		t.Fatalf("preedit child ok=%v err=%v focus=%#v nodes=%s", ok, err, preedit.Focus(), preeditJSON)
	}
}

func TestComposeTreeHiddenProjectionDoesNotInspectPrivateValues(t *testing.T) {
	document, ok, err := ComposeTree(TreeProjectionInput{Visible: false, Window: TreeWindow{Name: string([]byte{0xff})}})
	if err != nil || ok || document.ProviderID() != 0 {
		t.Fatalf("hidden projection document=%#v ok=%v err=%v", document, ok, err)
	}
}

func TestComposeTreeCloseTransferAndWindowIsolation(t *testing.T) {
	compose := func(projection, generation uint64, panes ...struct {
		object uint64
		text   string
	}) Document {
		window := treeID(NodeKindWindow, projection, 1, 0)
		tab := treeID(NodeKindTab, projection, 2, 0)
		treePanes := make([]TreePane, len(panes))
		for index, pane := range panes {
			id := treeID(NodeKindPane, projection, pane.object, 1)
			treePanes[index] = TreePane{Terminal: treeTerminal(9, generation, window, id, "terminal", pane.text)}
		}
		focus := NodeID{}
		if len(panes) > 0 {
			focus = treeID(NodeKindPane, projection, panes[len(panes)-1].object, 1)
		}
		document, ok, err := ComposeTree(TreeProjectionInput{ProviderID: 9, Generation: generation, Visible: true, FocusedPane: focus, Window: TreeWindow{ID: window, Focused: len(panes) > 0, Tabs: []TreeTab{{ID: tab, Active: true, Panes: treePanes}}}})
		if err != nil || !ok {
			t.Fatalf("compose projection=%d ok=%v err=%v", projection, ok, err)
		}
		return document
	}
	left := compose(30, 1, struct {
		object uint64
		text   string
	}{7, "LEFT"}, struct {
		object uint64
		text   string
	}{8, "MOVE-ME"})
	afterClose := compose(30, 2, struct {
		object uint64
		text   string
	}{7, "LEFT"})
	destination := compose(31, 1, struct {
		object uint64
		text   string
	}{8, "MOVE-ME"})
	leftJSON, _ := json.Marshal(left.Nodes())
	closeJSON, _ := json.Marshal(afterClose.Nodes())
	destinationJSON, _ := json.Marshal(destination.Nodes())
	if !strings.Contains(string(leftJSON), "MOVE-ME") || strings.Contains(string(closeJSON), "MOVE-ME") || strings.Contains(string(destinationJSON), "LEFT") {
		t.Fatalf("isolation before=%s after=%s destination=%s", leftJSON, closeJSON, destinationJSON)
	}
	if destination.Focus().Object != 8 || destination.Focus().Projection != 31 {
		t.Fatalf("destination focus=%#v", destination.Focus())
	}
	moving := treeID(NodeKindPane, 30, 8, 1)
	value, err := NewRange(left, moving, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := value.Text(afterClose); !errors.Is(err, ErrStaleRange) {
		t.Fatalf("closed-node range err=%v", err)
	}
}

func TestComposeTreeRejectsAmbiguousOrStaleFocusInputs(t *testing.T) {
	window := treeID(NodeKindWindow, 40, 1, 0)
	tab := treeID(NodeKindTab, 40, 2, 0)
	pane := treeID(NodeKindPane, 40, 3, 1)
	base := TreeProjectionInput{ProviderID: 1, Generation: 1, Visible: true, FocusedPane: pane, Window: TreeWindow{ID: window, Focused: true, Tabs: []TreeTab{{ID: tab, Active: true, Panes: []TreePane{{Terminal: treeTerminal(1, 1, window, pane, "pane", "x")}}}}}}
	for _, mutate := range []func(*TreeProjectionInput){
		func(value *TreeProjectionInput) {
			value.Window.Tabs = append(value.Window.Tabs, TreeTab{ID: treeID(NodeKindTab, 40, 9, 0), Active: true})
		},
		func(value *TreeProjectionInput) { value.FocusedPane = treeID(NodeKindPane, 40, 99, 1) },
		func(value *TreeProjectionInput) {
			value.Search = &TreeInput{ID: treeID(NodeKindInput, 40, 5, 0), Parent: pane, Role: RoleTextField}
		},
		func(value *TreeProjectionInput) {
			value.Preedit = &TreeInput{ID: treeID(NodeKindInput, 40, 5, 0), Parent: pane, Role: RoleTextField}
		},
	} {
		candidate := base
		candidate.Window.Tabs = append([]TreeTab(nil), base.Window.Tabs...)
		mutate(&candidate)
		if _, _, err := ComposeTree(candidate); err == nil {
			t.Fatalf("accepted malformed candidate=%#v", candidate)
		}
	}
}

func TestComposeTreeAppliesGlobalNodeAndRowBounds(t *testing.T) {
	window := treeID(NodeKindWindow, 41, 1, 0)
	tab := treeID(NodeKindTab, 41, 2, 0)
	panes := make([]TreePane, 300)
	for index := range panes {
		pane := treeID(NodeKindPane, 41, uint64(index+10), 1)
		panes[index] = TreePane{Terminal: treeTerminal(2, 1, window, pane, "pane", "x")}
	}
	focused := panes[len(panes)-1].Terminal.PaneID
	preedit := TreeInput{ID: treeID(NodeKindInput, 41, 9, 1), Parent: focused, Role: RoleTextField, Text: "out-of-budget"}
	document, ok, err := ComposeTree(TreeProjectionInput{ProviderID: 2, Generation: 1, Visible: true, FocusedPane: focused, Preedit: &preedit, Window: TreeWindow{ID: window, Focused: true, Tabs: []TreeTab{{ID: tab, Active: true, Panes: panes}}}})
	if err != nil || !ok || !document.Truncated() || len(document.Nodes()) != MaxNodes || document.Focus() != (NodeID{}) {
		t.Fatalf("node bound ok=%v err=%v truncated=%v nodes=%d focus=%#v", ok, err, document.Truncated(), len(document.Nodes()), document.Focus())
	}
	if _, exists := document.Node(preedit.ID); exists {
		t.Fatal("out-of-budget preedit displaced its focused pane")
	}

	rowTerminal := func(pane NodeID) TerminalProjectionInput {
		cells := make([]TerminalCell, 400)
		wrapped := make([]bool, 400)
		for index := range cells {
			cells[index] = TerminalCell{Text: "x"}
		}
		return TerminalProjectionInput{ProviderID: 2, Generation: 2, RootID: window, PaneID: pane, PaneName: "pane", Cols: 1, Rows: 400, Cells: cells, Wrapped: wrapped, CellWidth: 8, CellHeight: 16, Clip: Rect{Width: 8, Height: 6400}}
	}
	first := treeID(NodeKindPane, 41, 500, 1)
	second := treeID(NodeKindPane, 41, 501, 1)
	document, ok, err = ComposeTree(TreeProjectionInput{ProviderID: 2, Generation: 2, Visible: true, FocusedPane: second, Window: TreeWindow{ID: window, Focused: true, Tabs: []TreeTab{{ID: tab, Active: true, Panes: []TreePane{{Terminal: rowTerminal(first)}, {Terminal: rowTerminal(second)}}}}}})
	if err != nil || !ok || !document.Truncated() || document.RowCount() != MaxRows || document.Focus() != second {
		t.Fatalf("row bound ok=%v err=%v truncated=%v rows=%d focus=%#v", ok, err, document.Truncated(), document.RowCount(), document.Focus())
	}
}

func TestComposeTreeOmitsStalePreeditAtomically(t *testing.T) {
	window := treeID(NodeKindWindow, 42, 1, 0)
	tab := treeID(NodeKindTab, 42, 2, 0)
	pane := treeID(NodeKindPane, 42, 3, 1)
	staleParent := pane
	staleParent.Activation++
	document, ok, err := ComposeTree(TreeProjectionInput{
		ProviderID: 1, Generation: 1, Visible: true, FocusedPane: pane,
		Window:  TreeWindow{ID: window, Focused: true, Tabs: []TreeTab{{ID: tab, Active: true, Panes: []TreePane{{Terminal: treeTerminal(1, 1, window, pane, "pane", "safe")}}}}},
		Preedit: &TreeInput{ID: treeID(NodeKindInput, 42, 9, 1), Parent: staleParent, Role: RoleTextField, Text: "stale-secret"},
	})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	encoded, _ := json.Marshal(document.Nodes())
	if document.Focus() != pane || strings.Contains(string(encoded), "stale-secret") {
		t.Fatalf("focus=%#v nodes=%s", document.Focus(), encoded)
	}
}

func TestComposeTreeSharesGlobalGraphemeAndByteBudgetsWithInputs(t *testing.T) {
	window := treeID(NodeKindWindow, 43, 1, 0)
	tab := treeID(NodeKindTab, 43, 2, 0)
	pane := treeID(NodeKindPane, 43, 3, 1)
	modalID := treeID(NodeKindInput, 43, 4, 1)
	base := TreeProjectionInput{
		ProviderID: 1, Generation: 1, Visible: true, FocusedPane: pane,
		Window: TreeWindow{ID: window, Focused: true, Tabs: []TreeTab{{ID: tab, Active: true, Panes: []TreePane{{Terminal: treeTerminal(1, 1, window, pane, "pane", "x")}}}}},
	}
	base.Modal = &TreeInput{ID: modalID, Parent: window, Role: RoleDialog, Text: strings.Repeat("g", MaxGraphemes+10)}
	document, ok, err := ComposeTree(base)
	if err != nil || !ok || !document.Truncated() || document.GraphemeCount() != MaxGraphemes {
		t.Fatalf("grapheme budget ok=%v err=%v truncated=%v graphemes=%d", ok, err, document.Truncated(), document.GraphemeCount())
	}
	base.Generation = 2
	base.Window.Tabs[0].Panes[0].Terminal.Generation = 2
	largeCluster := "a" + strings.Repeat("\u0301", (MaxUTF8Bytes-2)/2)
	base.Modal = &TreeInput{ID: modalID, Parent: window, Role: RoleDialog, Text: largeCluster}
	document, ok, err = ComposeTree(base)
	if err != nil || !ok || !document.Truncated() || document.UTF8Bytes() > MaxUTF8Bytes {
		t.Fatalf("byte budget ok=%v err=%v truncated=%v bytes=%d", ok, err, document.Truncated(), document.UTF8Bytes())
	}
	modalNode, exists := document.Node(modalID)
	if !exists || modalNode.Text != "" {
		t.Fatalf("oversized indivisible grapheme node=%#v exists=%v", modalNode, exists)
	}
}

func TestComposeTreeTabSwitchAndWindowFocusTransition(t *testing.T) {
	window := treeID(NodeKindWindow, 44, 1, 0)
	tabA := treeID(NodeKindTab, 44, 2, 0)
	tabB := treeID(NodeKindTab, 44, 3, 0)
	paneA := treeID(NodeKindPane, 44, 4, 1)
	paneB := treeID(NodeKindPane, 44, 5, 1)
	input := TreeProjectionInput{ProviderID: 1, Generation: 1, Visible: true, FocusedPane: paneA, Window: TreeWindow{ID: window, Focused: true, Tabs: []TreeTab{
		{ID: tabA, Active: true, Panes: []TreePane{{Terminal: treeTerminal(1, 1, window, paneA, "pane", "ALPHA")}}},
		{ID: tabB, Panes: []TreePane{{Terminal: treeTerminal(1, 2, window, paneB, "pane", "BETA")}}},
	}}}
	first, ok, err := ComposeTree(input)
	if err != nil || !ok || first.Focus() != paneA {
		t.Fatalf("first ok=%v err=%v focus=%#v", ok, err, first.Focus())
	}
	input.Generation = 2
	input.Window.Focused = false
	input.FocusedPane = paneB
	input.Window.Tabs[0].Active = false
	input.Window.Tabs[1].Active = true
	second, ok, err := ComposeTree(input)
	if err != nil || !ok || second.Focus() != (NodeID{}) {
		t.Fatalf("second ok=%v err=%v focus=%#v", ok, err, second.Focus())
	}
	encoded, _ := json.Marshal(second.Nodes())
	if strings.Contains(string(encoded), "ALPHA") || !strings.Contains(string(encoded), "BETA") {
		t.Fatalf("switched nodes=%s", encoded)
	}
}

func TestComposeTreeConcurrentDetachedValues(t *testing.T) {
	window := treeID(NodeKindWindow, 45, 1, 0)
	tab := treeID(NodeKindTab, 45, 2, 0)
	pane := treeID(NodeKindPane, 45, 3, 1)
	input := TreeProjectionInput{ProviderID: 1, Generation: 1, Visible: true, FocusedPane: pane, Window: TreeWindow{ID: window, Focused: true, Tabs: []TreeTab{{ID: tab, Active: true, Panes: []TreePane{{Terminal: treeTerminal(1, 1, window, pane, "pane", "safe")}}}}}}
	errorsCh := make(chan error, 8)
	for worker := 0; worker < cap(errorsCh); worker++ {
		go func() {
			for iteration := 0; iteration < 50; iteration++ {
				document, ok, err := ComposeTree(input)
				if err == nil && (!ok || document.Focus() != pane) {
					err = ErrInvalidProjection
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
