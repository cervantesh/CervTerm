package accessibility

import (
	"strings"
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

type TreeWindow struct {
	ID      NodeID
	Name    string
	Focused bool
	Tabs    []TreeTab
}

type TreeTab struct {
	ID     NodeID
	Name   string
	Active bool
	Panes  []TreePane
}

type TreePane struct {
	Terminal TerminalProjectionInput
}

type TreeInput struct {
	ID        NodeID
	Parent    NodeID
	Role      Role
	Name      string
	Text      string
	Caret     *int
	Selection *Span
}

type TreeProjectionInput struct {
	ProviderID  uint64
	Generation  uint64
	Visible     bool
	Window      TreeWindow
	FocusedPane NodeID
	Modal       *TreeInput
	Search      *TreeInput
	Preedit     *TreeInput
}

// ComposeTree creates one immutable document for one visible native-window
// projection. Hidden windows/workspaces return ok=false without inspecting pane
// content. Only the active tab is traversed, so hidden-tab terminal values never
// enter the semantic document.
func ComposeTree(input TreeProjectionInput) (document Document, ok bool, err error) {
	if !input.Visible {
		return Document{}, false, nil
	}
	if input.ProviderID == 0 || input.Generation == 0 || input.Generation == ^uint64(0) || !input.Window.ID.Valid() || input.Window.ID.Kind != NodeKindWindow {
		return Document{}, false, ErrInvalidProjection
	}
	active, found, err := activeTreeTab(input.Window)
	if err != nil {
		return Document{}, false, err
	}
	if !found {
		return Document{}, false, nil
	}
	if active.ID.Kind != NodeKindTab || active.ID.Projection != input.Window.ID.Projection {
		return Document{}, false, ErrInvalidProjection
	}
	if len(active.Panes) == 0 {
		return Document{}, false, nil
	}

	winner := winningTreeInput(input)
	focusSurface := input.FocusedPane
	if winner != nil {
		focusSurface = winner.ID
	}
	reserve := 0
	if winner != nil {
		reserve++
	}
	includePreedit := input.Preedit != nil && input.Preedit.Parent == focusSurface
	if includePreedit && (winner == nil || winner.Role == RoleTextField) {
		focusedIndex := treePaneIndex(active.Panes, input.FocusedPane)
		includePreedit = focusedIndex >= 0 && focusedIndex < MaxNodes-2-reserve-1
	}
	if includePreedit {
		reserve++
	}
	nodes := make([]NodeDraft, 0, min(MaxNodes, 2+len(active.Panes)+reserve))
	nodes = append(nodes,
		NodeDraft{ID: input.Window.ID, Role: RoleWindow, Name: input.Window.Name},
		NodeDraft{ID: active.ID, Parent: input.Window.ID, Role: RoleTab, Name: active.Name},
	)
	truncated := false
	rows, graphemeCount, byteCount := 0, 0, len(input.Window.Name)+len(active.Name)
	focusedPanePresent := false
	for _, paneInput := range active.Panes {
		if len(nodes) >= MaxNodes-reserve || rows >= MaxRows || graphemeCount >= MaxGraphemes || byteCount >= MaxUTF8Bytes {
			truncated = true
			break
		}
		terminal := paneInput.Terminal
		if terminal.ProviderID != input.ProviderID || terminal.Generation != input.Generation || terminal.RootID != input.Window.ID || terminal.PaneID.Projection != input.Window.ID.Projection {
			return Document{}, false, ErrInvalidProjection
		}
		pane, paneTruncated, projectErr := projectTerminalNodeWithBudget(terminal, MaxRows-rows, MaxUTF8Bytes-byteCount-len(terminal.PaneName), MaxGraphemes-graphemeCount)
		if projectErr != nil {
			return Document{}, false, projectErr
		}
		pane.Parent = active.ID
		nodes = append(nodes, pane)
		truncated = truncated || paneTruncated
		rows += len(pane.Rows)
		byteCount += len(pane.Name)
		for rowIndex, row := range pane.Rows {
			if rowIndex > 0 && !pane.Rows[rowIndex-1].SoftWrapped {
				byteCount++
				graphemeCount++
			}
			byteCount += len(row.Text)
			iterator := graphemes.FromString(row.Text)
			for iterator.Next() {
				graphemeCount++
			}
		}
		if pane.ID == input.FocusedPane {
			focusedPanePresent = true
		}
	}
	if !focusedPanePresent && !truncated {
		return Document{}, false, ErrInvalidProjection
	}
	if winner != nil && winner.Role == RoleTextField && !focusedPanePresent {
		winner = nil
	}

	focus := NodeID{}
	if input.Window.Focused && focusedPanePresent {
		focus = input.FocusedPane
	}
	focusSurface = input.FocusedPane
	if winner != nil {
		if (input.Modal != nil && winner.Role != RoleDialog) || (input.Modal == nil && winner.Role != RoleTextField) {
			return Document{}, false, ErrInvalidProjection
		}
		if err := validateTreeInput(*winner, input.Window.ID, input.FocusedPane, nodes, true); err != nil {
			return Document{}, false, err
		}
		node, inputTruncated := boundedTreeInputNode(*winner, MaxRows-rows, max(0, MaxUTF8Bytes-byteCount-len(winner.Name)), max(0, MaxGraphemes-graphemeCount))
		nodes = append(nodes, node)
		truncated = truncated || inputTruncated
		rows, graphemeCount, byteCount = addTreeNodeBudget(node, rows, graphemeCount, byteCount)
		focusSurface = winner.ID
		if input.Window.Focused {
			focus = winner.ID
		}
	}
	includePreedit = includePreedit && input.Preedit != nil && input.Preedit.Parent == focusSurface && treeNodePresent(nodes, focusSurface)
	if includePreedit {
		if err := validateTreeInput(*input.Preedit, input.Window.ID, focusSurface, nodes, false); err != nil {
			return Document{}, false, err
		}
		node, inputTruncated := boundedTreeInputNode(*input.Preedit, MaxRows-rows, max(0, MaxUTF8Bytes-byteCount-len(input.Preedit.Name)), max(0, MaxGraphemes-graphemeCount))
		nodes = append(nodes, node)
		truncated = truncated || inputTruncated
		rows, graphemeCount, byteCount = addTreeNodeBudget(node, rows, graphemeCount, byteCount)
	}
	if input.Window.Focused && focus == (NodeID{}) && !truncated {
		return Document{}, false, ErrInvalidProjection
	}
	document, err = NewDocument(DocumentDraft{
		ProviderID: input.ProviderID,
		Generation: input.Generation,
		Nodes:      nodes,
		Focus:      focus,
		Truncated:  truncated,
	})
	if err != nil {
		return Document{}, false, err
	}
	return document, true, nil
}

func activeTreeTab(window TreeWindow) (TreeTab, bool, error) {
	var active TreeTab
	found := false
	for _, tab := range window.Tabs {
		if !tab.Active {
			continue
		}
		if found {
			return TreeTab{}, false, ErrInvalidProjection
		}
		active, found = tab, true
	}
	return active, found, nil
}

func treePaneIndex(panes []TreePane, id NodeID) int {
	for index, pane := range panes {
		if pane.Terminal.PaneID == id {
			return index
		}
	}
	return -1
}

func winningTreeInput(input TreeProjectionInput) *TreeInput {
	if input.Modal != nil {
		return input.Modal
	}
	if input.Search != nil {
		return input.Search
	}
	return nil
}

func treeNodePresent(nodes []NodeDraft, id NodeID) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func validateTreeInput(input TreeInput, window, expectedParent NodeID, nodes []NodeDraft, dialogAllowed bool) error {
	if !input.ID.Valid() || input.ID.Kind != NodeKindInput || input.ID.Projection != window.Projection || input.ID.Activation == 0 || !utf8.ValidString(input.Text) || strings.ContainsAny(input.Text, "\r\n") || len(input.Text) > MaxUTF8Bytes {
		return ErrInvalidProjection
	}
	if input.Role != RoleDialog && input.Role != RoleTextField {
		return ErrInvalidProjection
	}
	parentPresent := treeNodePresent(nodes, input.Parent)
	if !parentPresent {
		return ErrInvalidProjection
	}
	if input.Role == RoleDialog {
		if !dialogAllowed || input.Parent != window {
			return ErrInvalidProjection
		}
	} else if input.Parent != expectedParent {
		return ErrInvalidProjection
	}
	return nil
}

func boundedTreeInputNode(input TreeInput, rowBudget, byteBudget, graphemeBudget int) (NodeDraft, bool) {
	node := NodeDraft{ID: input.ID, Parent: input.Parent, Role: input.Role, Name: input.Name}
	truncated := false
	if rowBudget > 0 {
		var text strings.Builder
		iterator := graphemes.FromString(input.Text)
		count := 0
		for iterator.Next() {
			cluster := iterator.Value()
			if count == graphemeBudget || len(cluster) > byteBudget-text.Len() {
				truncated = true
				break
			}
			text.WriteString(cluster)
			count++
		}
		node.Rows = []RowDraft{{Text: text.String()}}
	} else {
		truncated = true
	}
	if input.Caret != nil {
		value := *input.Caret
		node.Caret = &value
	}
	if input.Selection != nil {
		value := *input.Selection
		node.Selection = &value
	}
	return node, truncated
}

func addTreeNodeBudget(node NodeDraft, rows, graphemeCount, byteCount int) (int, int, int) {
	byteCount += len(node.Name)
	rows += len(node.Rows)
	for rowIndex, row := range node.Rows {
		if rowIndex > 0 && !node.Rows[rowIndex-1].SoftWrapped {
			byteCount++
			graphemeCount++
		}
		byteCount += len(row.Text)
		iterator := graphemes.FromString(row.Text)
		for iterator.Next() {
			graphemeCount++
		}
	}
	return rows, graphemeCount, byteCount
}
