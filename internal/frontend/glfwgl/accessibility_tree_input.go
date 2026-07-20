//go:build glfw

package glfwgl

import (
	"cervterm/internal/accessibility"
	termmux "cervterm/internal/mux"
)

type treeAccessibilityCapture struct {
	ProviderID       uint64
	Generation       uint64
	ProjectionID     uint64
	WorkspaceVisible bool
	Window           termmux.WindowView
	PaneActivations  map[termmux.PaneID]uint64
	Panes            map[termmux.PaneID]terminalAccessibilityCapture
	SafeWindowName   string
	SafeTabNames     map[termmux.TabID]string
	Modal            *accessibility.TreeInput
	Search           *accessibility.TreeInput
	Preedit          *accessibility.TreeInput
}

func buildTreeAccessibilityInput(capture treeAccessibilityCapture) (accessibility.TreeProjectionInput, error) {
	if !capture.WorkspaceVisible {
		return accessibility.TreeProjectionInput{Visible: false}, nil
	}
	if capture.ProviderID == 0 || capture.Generation == 0 || capture.ProjectionID == 0 || capture.Window.ID == 0 {
		return accessibility.TreeProjectionInput{}, accessibility.ErrInvalidProjection
	}
	windowID := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: capture.ProjectionID, Object: uint64(capture.Window.ID)}
	windowName := capture.SafeWindowName
	if windowName == "" {
		windowName = "CervTerm window"
	}
	result := accessibility.TreeProjectionInput{
		ProviderID: capture.ProviderID, Generation: capture.Generation, Visible: true,
		Window: accessibility.TreeWindow{ID: windowID, Name: windowName, Focused: capture.Window.Active},
		Modal:  cloneTreeAccessibilityInput(capture.Modal), Search: cloneTreeAccessibilityInput(capture.Search), Preedit: cloneTreeAccessibilityInput(capture.Preedit),
	}
	for _, tab := range capture.Window.Tabs {
		if !tab.Active {
			continue
		}
		tabName := capture.SafeTabNames[tab.ID]
		if tabName == "" {
			tabName = "Terminal tab"
		}
		treeTab := accessibility.TreeTab{
			ID:   accessibility.NodeID{Kind: accessibility.NodeKindTab, Projection: capture.ProjectionID, Object: uint64(tab.ID)},
			Name: tabName, Active: true, Panes: make([]accessibility.TreePane, 0, len(tab.Panes)),
		}
		for _, paneID := range tab.Panes {
			activation := capture.PaneActivations[paneID]
			paneCapture, exists := capture.Panes[paneID]
			if activation == 0 || !exists {
				return accessibility.TreeProjectionInput{}, accessibility.ErrInvalidProjection
			}
			accessibilityPaneID := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: capture.ProjectionID, Object: uint64(paneID), Activation: activation}
			paneCapture.ProviderID = capture.ProviderID
			paneCapture.Generation = capture.Generation
			paneCapture.RootID = windowID
			paneCapture.PaneID = accessibilityPaneID
			paneCapture.RootName = windowName
			paneCapture.PaneName = "terminal"
			terminal, err := buildTerminalAccessibilityInput(paneCapture)
			if err != nil {
				return accessibility.TreeProjectionInput{}, err
			}
			treeTab.Panes = append(treeTab.Panes, accessibility.TreePane{Terminal: terminal})
			if tab.Focused == paneID {
				result.FocusedPane = accessibilityPaneID
			}
		}
		result.Window.Tabs = append(result.Window.Tabs, treeTab)
	}
	return result, nil
}

func cloneTreeAccessibilityInput(input *accessibility.TreeInput) *accessibility.TreeInput {
	if input == nil {
		return nil
	}
	clone := *input
	if input.Caret != nil {
		value := *input.Caret
		clone.Caret = &value
	}
	if input.Selection != nil {
		value := *input.Selection
		clone.Selection = &value
	}
	return &clone
}
