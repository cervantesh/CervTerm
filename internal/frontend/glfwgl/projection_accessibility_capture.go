//go:build glfw

package glfwgl

import (
	"fmt"

	"cervterm/internal/accessibility"
	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
	termsel "cervterm/internal/selection"
)

func (app *App) captureAccessibilityDocument(generation uint64) (accessibility.Document, bool, error) {
	return app.captureAccessibilityDocumentVisibility(generation, true)
}

func (app *App) captureAccessibilityDocumentVisibility(generation uint64, nativeVisible bool) (accessibility.Document, bool, error) {
	if app == nil || app.mux == nil || app.windowID == 0 || generation == 0 {
		return accessibility.Document{}, false, errProjectionAccessibilityInvalid
	}
	var window termmux.WindowView
	found := false
	for _, candidate := range app.mux.Windows() {
		if candidate.ID == app.windowID {
			window, found = candidate, true
			break
		}
	}
	if !found {
		return accessibility.Document{}, false, fmt.Errorf("accessibility projection window is unavailable")
	}
	activeWorkspace := app.mux.ActiveWorkspace()
	visible := nativeVisible && activeWorkspace.ID != 0 && window.Workspace == activeWorkspace.ID
	projection := uint64(window.ID)
	capture := treeAccessibilityCapture{
		ProviderID: projection, Generation: generation, ProjectionID: projection,
		WorkspaceVisible: visible, Window: window,
		PaneActivations: make(map[termmux.PaneID]uint64), Panes: make(map[termmux.PaneID]terminalAccessibilityCapture),
		SafeWindowName: "CervTerm window", SafeTabNames: make(map[termmux.TabID]string),
	}
	if !visible {
		root := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: projection, Object: uint64(window.ID)}
		document, err := accessibility.NewDocument(accessibility.DocumentDraft{
			ProviderID: projection, Generation: generation, Focus: root,
			Nodes: []accessibility.NodeDraft{{ID: root, Role: accessibility.RoleWindow, Name: "CervTerm window"}},
		})
		return document, err == nil, err
	}
	for _, tab := range window.Tabs {
		if !tab.Active {
			continue
		}
		capture.SafeTabNames[tab.ID] = "Terminal tab"
		for _, paneID := range tab.Panes {
			view, ok := app.mux.PaneView(paneID)
			if !ok {
				return accessibility.Document{}, false, fmt.Errorf("accessibility pane is unavailable")
			}
			activation := uint64(paneID)
			if activation == 0 {
				return accessibility.Document{}, false, accessibility.ErrInvalidProjection
			}
			capture.PaneActivations[paneID] = activation
			cellWidth, cellHeight := float64(app.cellW), float64(app.cellH)
			state := app.paneUI[paneID]
			if state != nil && state.font.cellW > 0 && state.font.cellH > 0 {
				cellWidth, cellHeight = float64(state.font.cellW), float64(state.font.cellH)
			}
			var selection *termsel.Range
			selected := selectionState{}
			if paneID == app.focusedPane {
				selected = app.selection
			} else if state != nil {
				selected = state.selection
			}
			if selected.active {
				value := termsel.Normalize(termsel.Range{Start: selected.start, End: selected.end})
				selection = &value
			}
			capture.Panes[paneID] = terminalAccessibilityCapture{
				Snapshot: view.Snapshot, PanePixels: view.Geometry.Pixels, CellWidth: cellWidth, CellHeight: cellHeight,
				Selection: selection, Bidi: app.cfg.Render.Bidi, Alternate: view.AlternateScreen,
			}
		}
		break
	}
	windowNode := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: projection, Object: uint64(window.ID)}
	focusedPane := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: projection, Object: uint64(app.focusedPane), Activation: uint64(app.focusedPane)}
	capture.Modal = modalAccessibilityInput(projection, windowNode, app.modal.Snapshot())
	capture.Search = searchAccessibilityInput(projection, focusedPane, app.search)
	preeditParent := focusedPane
	preedit := app.composition.snapshot()
	if preedit.Active {
		switch preedit.Target.Kind {
		case ime.TargetSearch:
			if capture.Search != nil {
				preeditParent = capture.Search.ID
			}
		case ime.TargetModal:
			if capture.Modal != nil {
				preeditParent = capture.Modal.ID
			}
		}
	}
	capture.Preedit = preeditAccessibilityInput(projection, preeditParent, preedit)
	input, err := buildTreeAccessibilityInput(capture)
	if err != nil {
		return accessibility.Document{}, false, err
	}
	return accessibility.ComposeTree(input)
}
