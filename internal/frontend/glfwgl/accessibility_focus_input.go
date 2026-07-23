//go:build glfw

package glfwgl

import (
	"cervterm/internal/accessibility"
	"cervterm/internal/ime"
	"cervterm/internal/modal"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

const (
	accessibilityModalObject   = uint64(1)<<63 | 1
	accessibilitySearchObject  = uint64(1)<<63 | 2
	accessibilityPreeditObject = uint64(1)<<63 | 3
)

func modalAccessibilityInput(projection uint64, window accessibility.NodeID, state modal.State) *accessibility.TreeInput {
	if !state.Mode.Valid() || state.Activation == 0 || state.OpeningPane == 0 {
		return nil
	}
	text := string(state.Query)
	caret := accessibilityGraphemeCount(text)
	return &accessibility.TreeInput{
		ID:     accessibility.NodeID{Kind: accessibility.NodeKindInput, Projection: projection, Object: accessibilityModalObject, Activation: uint64(state.Activation)},
		Parent: window, Role: accessibility.RoleDialog, Name: accessibilityModalName(state.Mode), Text: text, Caret: &caret,
	}
}

func searchAccessibilityInput(projection uint64, pane accessibility.NodeID, search searchController) *accessibility.TreeInput {
	if !search.active || search.activation == 0 || pane.Kind != accessibility.NodeKindPane {
		return nil
	}
	text := string(search.query)
	caret := accessibilityGraphemeCount(text)
	return &accessibility.TreeInput{
		ID:     accessibility.NodeID{Kind: accessibility.NodeKindInput, Projection: projection, Object: accessibilitySearchObject, Activation: uint64(search.activation)},
		Parent: pane, Role: accessibility.RoleTextField, Name: "search", Text: text, Caret: &caret,
	}
}

func preeditAccessibilityInput(projection uint64, parent accessibility.NodeID, snapshot ime.Snapshot) *accessibility.TreeInput {
	if !snapshot.Active || snapshot.Generation == 0 || !snapshot.Target.Valid() || parent.Projection != projection || !accessibilityPreeditTargetMatches(parent, snapshot.Target) {
		return nil
	}
	caret, ok := accessibilityGraphemeOffset(snapshot.Text, snapshot.CursorRune)
	if !ok {
		return nil
	}
	start, startOK := accessibilityGraphemeOffset(snapshot.Text, snapshot.TargetRuneSpan.Start)
	end, endOK := accessibilityGraphemeOffset(snapshot.Text, snapshot.TargetRuneSpan.End)
	if !startOK || !endOK || end < start {
		return nil
	}
	selection := accessibility.Span{Start: start, End: end}
	return &accessibility.TreeInput{
		ID:     accessibility.NodeID{Kind: accessibility.NodeKindInput, Projection: projection, Object: accessibilityPreeditObject, Activation: snapshot.Generation},
		Parent: parent, Role: accessibility.RoleTextField, Name: "composition", Text: snapshot.Text, Caret: &caret, Selection: &selection,
	}
}

func accessibilityPreeditTargetMatches(parent accessibility.NodeID, target ime.Target) bool {
	if parent.Activation != target.Activation {
		return false
	}
	switch target.Kind {
	case ime.TargetPane:
		return parent.Kind == accessibility.NodeKindPane && parent.Object == target.ID
	case ime.TargetSearch:
		return parent.Kind == accessibility.NodeKindInput && parent.Object == accessibilitySearchObject
	case ime.TargetModal:
		return parent.Kind == accessibility.NodeKindInput && parent.Object == accessibilityModalObject
	default:
		return false
	}
}

func accessibilityModalName(mode modal.Mode) string {
	switch mode {
	case modal.ModeSearch:
		return "search"
	case modal.ModeCommandPalette:
		return "command palette"
	case modal.ModeQuickSelect:
		return "quick select"
	case modal.ModeLaunchMenu:
		return "launch menu"
	case modal.ModeTabSwitcher:
		return "tab switcher"
	case modal.ModeWorkspaceSwitcher:
		return "workspace switcher"
	case modal.ModeTabCloseConfirmation:
		return "close tab"
	default:
		return "dialog"
	}
}

func accessibilityGraphemeOffset(text string, runeOffset int) (int, bool) {
	runes := []rune(text)
	if runeOffset < 0 || runeOffset > len(runes) {
		return 0, false
	}
	return accessibilityGraphemeCount(string(runes[:runeOffset])), true
}

func accessibilityGraphemeCount(text string) int {
	count := 0
	iterator := graphemes.FromString(text)
	for iterator.Next() {
		count++
	}
	return count
}
