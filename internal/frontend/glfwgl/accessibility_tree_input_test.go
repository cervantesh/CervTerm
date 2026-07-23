//go:build glfw

package glfwgl

import (
	"encoding/json"
	"strings"
	"testing"

	"cervterm/internal/accessibility"
	"cervterm/internal/core"
	"cervterm/internal/ime"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"cervterm/internal/render"
)

func TestTreeAccessibilityInputTraversesOnlyActiveWorkspaceTab(t *testing.T) {
	window := termmux.WindowView{ID: 5, Active: true, Tabs: []termmux.TabView{
		{ID: 8, Title: "hidden-process-title-secret", Panes: []termmux.PaneID{99}},
		{ID: 9, Title: "active-process-title-secret", Focused: 12, Panes: []termmux.PaneID{11, 12}, Active: true},
	}}
	paneCapture := func(text rune, x int) terminalAccessibilityCapture {
		return terminalAccessibilityCapture{
			Snapshot:   render.Snapshot{Cols: 1, Rows: 1, Cells: []core.Cell{{Rune: text}}, Wrapped: []bool{false}, CursorVisible: true},
			PanePixels: termmux.PixelRect{X: x, Width: 8, Height: 16}, CellWidth: 8, CellHeight: 16,
		}
	}
	capture := treeAccessibilityCapture{
		ProviderID: 4, Generation: 2, ProjectionID: 50, WorkspaceVisible: true, Window: window,
		PaneActivations: map[termmux.PaneID]uint64{11: 3, 12: 4},
		Panes:           map[termmux.PaneID]terminalAccessibilityCapture{11: paneCapture('L', 0), 12: paneCapture('R', 8)},
		SafeWindowName:  "safe window", SafeTabNames: map[termmux.TabID]string{9: "safe tab"},
	}
	input, err := buildTreeAccessibilityInput(capture)
	if err != nil {
		t.Fatal(err)
	}
	capture.SafeTabNames[9] = "mutated"
	pane := capture.Panes[11]
	pane.Snapshot.Cells[0].Rune = 'X'
	capture.Panes[11] = pane
	document, ok, err := accessibility.ComposeTree(input)
	if err != nil || !ok {
		t.Fatalf("compose ok=%v err=%v", ok, err)
	}
	encoded, _ := json.Marshal(document.Nodes())
	text := string(encoded)
	if document.Focus().Object != 12 || !strings.Contains(text, "safe tab") || !strings.Contains(text, "L") || !strings.Contains(text, "R") {
		t.Fatalf("document focus=%#v nodes=%s", document.Focus(), encoded)
	}
	for _, secret := range []string{"hidden-process-title-secret", "active-process-title-secret", "mutated"} {
		if strings.Contains(text, secret) {
			t.Fatalf("leaked %q in %s", secret, text)
		}
	}
}

func TestTreeAccessibilityInputHiddenWorkspaceDoesNotRequirePaneCaptures(t *testing.T) {
	input, err := buildTreeAccessibilityInput(treeAccessibilityCapture{
		WorkspaceVisible: false,
		Window:           termmux.WindowView{ID: 1, Tabs: []termmux.TabView{{ID: 1, Active: true, Panes: []termmux.PaneID{99}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := accessibility.ComposeTree(input); err != nil || ok {
		t.Fatalf("hidden compose ok=%v err=%v", ok, err)
	}
}

func TestAccessibilityFocusInputsUseStableActivationsAndGraphemeOffsets(t *testing.T) {
	window := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: 60, Object: 1}
	pane := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 60, Object: 7, Activation: 9}
	modalInput := modalAccessibilityInput(60, window, modal.State{Mode: modal.ModeCommandPalette, Activation: 4, OpeningPane: 7, Query: []rune("e\u0301")})
	if modalInput == nil || modalInput.ID.Activation != 4 || modalInput.ID.Object != accessibilityModalObject || modalInput.Caret == nil || *modalInput.Caret != 1 {
		t.Fatalf("modal=%#v", modalInput)
	}
	searchInput := searchAccessibilityInput(60, pane, searchController{active: true, activation: 5, query: []rune("👩‍💻")})
	if searchInput == nil || searchInput.ID.Activation != 5 || searchInput.ID.Object != accessibilitySearchObject || *searchInput.Caret != 1 {
		t.Fatalf("search=%#v", searchInput)
	}
	preedit := preeditAccessibilityInput(60, pane, ime.Snapshot{
		Active: true, Target: ime.Target{Kind: ime.TargetPane, ID: 7, Activation: 9}, Generation: 6,
		Text: "e\u0301好", CursorRune: 2, TargetRuneSpan: ime.Span{Start: 2, End: 3},
	})
	if preedit == nil || preedit.ID.Activation != 6 || preedit.ID.Object != accessibilityPreeditObject || *preedit.Caret != 1 || *preedit.Selection != (accessibility.Span{Start: 1, End: 2}) {
		t.Fatalf("preedit=%#v", preedit)
	}
	stale := pane
	stale.Activation++
	if preeditAccessibilityInput(60, stale, ime.Snapshot{Active: true, Target: ime.Target{Kind: ime.TargetPane, ID: 7, Activation: 9}, Generation: 6}) != nil {
		t.Fatal("stale preedit target was accepted")
	}
}

func TestAccessibilityPreeditMatchesModalAndSearchTargets(t *testing.T) {
	modalParent := accessibility.NodeID{Kind: accessibility.NodeKindInput, Projection: 70, Object: accessibilityModalObject, Activation: 11}
	searchParent := accessibility.NodeID{Kind: accessibility.NodeKindInput, Projection: 70, Object: accessibilitySearchObject, Activation: 12}
	if value := preeditAccessibilityInput(70, modalParent, ime.Snapshot{Active: true, Target: ime.Target{Kind: ime.TargetModal, ID: 7, Activation: 11}, Generation: 2}); value == nil {
		t.Fatal("modal preedit target was rejected")
	}
	if value := preeditAccessibilityInput(70, searchParent, ime.Snapshot{Active: true, Target: ime.Target{Kind: ime.TargetSearch, ID: 7, Activation: 12}, Generation: 3}); value == nil {
		t.Fatal("search preedit target was rejected")
	}
}

func TestAccessibilitySearchIdentityTracksActivationNotRevision(t *testing.T) {
	pane := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 80, Object: 7, Activation: 3}
	first := searchAccessibilityInput(80, pane, searchController{active: true, activation: 5, query: []rune("first")})
	updated := searchAccessibilityInput(80, pane, searchController{active: true, activation: 5, query: []rune("updated")})
	reopened := searchAccessibilityInput(80, pane, searchController{active: true, activation: 6, query: []rune("first")})
	if first == nil || updated == nil || reopened == nil || first.ID != updated.ID || first.ID == reopened.ID || updated.Text != "updated" {
		t.Fatalf("first=%#v updated=%#v reopened=%#v", first, updated, reopened)
	}
	if closed := searchAccessibilityInput(80, pane, searchController{}); closed != nil {
		t.Fatalf("closed search=%#v", closed)
	}
}

func TestAccessibilityPreeditRejectsInactiveWrongProjectionAndMalformedSpans(t *testing.T) {
	pane := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 81, Object: 7, Activation: 3}
	target := ime.Target{Kind: ime.TargetPane, ID: 7, Activation: 3}
	for _, snapshot := range []ime.Snapshot{
		{Target: target, Generation: 1},
		{Active: true, Target: target, Generation: 1, Text: "x", CursorRune: 2},
		{Active: true, Target: target, Generation: 1, Text: "x", TargetRuneSpan: ime.Span{Start: 1, End: 0}},
	} {
		if value := preeditAccessibilityInput(81, pane, snapshot); value != nil {
			t.Fatalf("accepted snapshot=%#v value=%#v", snapshot, value)
		}
	}
	if value := preeditAccessibilityInput(82, pane, ime.Snapshot{Active: true, Target: target, Generation: 1}); value != nil {
		t.Fatalf("wrong projection=%#v", value)
	}
}
