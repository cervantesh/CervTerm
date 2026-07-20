package mux

import "fmt"

// FreshSessionSnapshot is a detached, secret-free description of topology and
// fresh local launch intent. Runtime IDs, environment, PTY state and cells are absent.
type FreshSessionSnapshot struct {
	ActiveWorkspace int
	Workspaces      []FreshWorkspace
}

type FreshWorkspace struct {
	Name         string
	ActiveWindow int
	Windows      []FreshWindow
}

type FreshWindow struct {
	Title     string
	ActiveTab int
	Tabs      []FreshTab
}

type FreshTab struct {
	Title       string
	FocusedLeaf int
	Root        FreshNode
}

type FreshNode struct {
	Type   string
	Launch *FreshLaunch
	Axis   SplitAxis
	Ratio  SplitRatio
	First  *FreshNode
	Second *FreshNode
}

type FreshLaunch struct {
	TargetID string
	Program  string
	Args     []string
	CWD      string
}

func (m *Mux) FreshSessionSnapshot() (FreshSessionSnapshot, error) {
	if err := m.model.CheckInvariants(); err != nil {
		return FreshSessionSnapshot{}, err
	}
	if len(m.model.workspaces) == 0 {
		return FreshSessionSnapshot{}, ErrInvalidRestore
	}
	out := FreshSessionSnapshot{ActiveWorkspace: -1, Workspaces: make([]FreshWorkspace, len(m.model.workspaces))}
	for workspaceIndex := range m.model.workspaces {
		workspace := &m.model.workspaces[workspaceIndex]
		fresh := FreshWorkspace{Name: workspace.name, ActiveWindow: -1, Windows: make([]FreshWindow, len(workspace.windows))}
		if workspace.id == m.model.activeWorkspace {
			out.ActiveWorkspace = workspaceIndex
		}
		for windowIndex, windowID := range workspace.windows {
			window := m.model.windowByID(windowID)
			if window == nil || len(window.tabs) == 0 {
				return FreshSessionSnapshot{}, invariantError("fresh snapshot window %d is unavailable", windowID)
			}
			if windowID == workspace.active {
				fresh.ActiveWindow = windowIndex
			}
			freshWindow := FreshWindow{Title: window.title, ActiveTab: -1, Tabs: make([]FreshTab, len(window.tabs))}
			for tabIndex := range window.tabs {
				tab := &window.tabs[tabIndex]
				if tab.id == window.active {
					freshWindow.ActiveTab = tabIndex
				}
				leaves := paneIDs(tab.root)
				focused := indexPaneID(leaves, tab.focused)
				if focused < 0 {
					return FreshSessionSnapshot{}, invariantError("fresh snapshot tab %d has no focused leaf", tab.id)
				}
				root, err := m.freshNode(tab.root)
				if err != nil {
					return FreshSessionSnapshot{}, fmt.Errorf("fresh snapshot tab %d: %w", tab.id, err)
				}
				freshWindow.Tabs[tabIndex] = FreshTab{Title: tab.title, FocusedLeaf: focused, Root: root}
			}
			if freshWindow.ActiveTab < 0 {
				return FreshSessionSnapshot{}, invariantError("fresh snapshot window %d has no active tab", windowID)
			}
			fresh.Windows[windowIndex] = freshWindow
		}
		if len(fresh.Windows) > 0 && fresh.ActiveWindow < 0 {
			return FreshSessionSnapshot{}, invariantError("fresh snapshot workspace %d has no active window", workspace.id)
		}
		out.Workspaces[workspaceIndex] = fresh
	}
	if out.ActiveWorkspace < 0 || len(out.Workspaces[out.ActiveWorkspace].Windows) == 0 {
		return FreshSessionSnapshot{}, ErrInvalidRestore
	}
	return out, nil
}

func (m *Mux) freshNode(source *node) (FreshNode, error) {
	if source == nil {
		return FreshNode{}, invariantError("nil topology node")
	}
	if source.isLeaf() {
		pane, ok := m.sessions.lookup(source.pane)
		if !ok {
			return FreshNode{}, invariantError("pane %d is not registry-owned", source.pane)
		}
		launch := pane.launch
		launch.Args = append([]string(nil), launch.Args...)
		if pane.cwd != "" {
			launch.CWD = pane.cwd
		}
		return FreshNode{Type: "pane", Launch: &launch}, nil
	}
	first, err := m.freshNode(source.first)
	if err != nil {
		return FreshNode{}, err
	}
	second, err := m.freshNode(source.second)
	if err != nil {
		return FreshNode{}, err
	}
	return FreshNode{Type: "split", Axis: source.axis, Ratio: source.ratio, First: &first, Second: &second}, nil
}

func indexPaneID(ids []PaneID, target PaneID) int {
	for index, id := range ids {
		if id == target {
			return index
		}
	}
	return -1
}
