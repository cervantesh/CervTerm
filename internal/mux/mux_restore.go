package mux

import (
	"errors"
	"fmt"

	"cervterm/internal/layoutrestore"
	"cervterm/internal/pty"
)

// RestoreWindowGeometry supplies the framebuffer and cell geometry for one
// blueprint window, ordered by workspace and then window traversal.
type RestoreWindowGeometry struct {
	Content PixelRect
	Metrics CellMetrics
}

// RestoreCandidate is an opaque, mux-owned unpublished restore transaction.
type RestoreCandidate struct {
	owner       *Mux
	model       *Model
	panes       []*pane
	windows     []WindowID
	paneMetrics map[PaneID]CellMetrics
	paneWindows map[PaneID]WindowID
	paneTabs    map[PaneID]TabID
	bounds      PixelRect
	committed   bool
	aborted     bool
}

type restoreBuild struct {
	candidate *RestoreCandidate
	specs     []SpawnSpec
}

// PrepareRestore validates and provisions a detached startup restore transaction.
func (m *Mux) PrepareRestore(blueprint layoutrestore.Blueprint, geometries []RestoreWindowGeometry) (*RestoreCandidate, error) {
	if m.pending != nil {
		return nil, ErrRestorePending
	}
	panes, reserved, started := m.sessions.activeCounts()
	if m.bootstrapped || panes != 0 || reserved != 0 || started != 0 {
		return nil, ErrRestorePrecondition
	}

	build, err := buildRestoreCandidate(m, blueprint.Snapshot(), geometries)
	if err != nil {
		return nil, err
	}
	candidate := build.candidate
	m.pending = candidate
	if err := m.provisionRestore(candidate, build.specs); err != nil {
		cleanupErr := m.abortRestore(candidate)
		return nil, errors.Join(err, cleanupErr)
	}
	return candidate, nil
}

// CommitRestore atomically publishes the exact pending restore transaction.
func (m *Mux) CommitRestore(candidate *RestoreCandidate) ([]Event, error) {
	if candidate == nil || candidate.owner != m || m.pending != candidate || candidate.aborted || candidate.committed {
		return nil, ErrInvalidRestore
	}
	if err := candidate.model.CheckInvariants(); err != nil {
		return nil, fmt.Errorf("commit restore: %w", err)
	}
	for _, p := range candidate.panes {
		owned, ok := m.sessions.lookup(p.id)
		if !ok || owned != p || p.state != PaneStateRunning || p.session == nil {
			return nil, ErrInvalidRestore
		}
	}
	ids := make([]PaneID, len(candidate.panes))
	for i, p := range candidate.panes {
		ids[i] = p.id
	}
	launchReaders, err := m.sessions.prepareStarts(ids)
	if err != nil {
		cleanupErr := m.abortRestore(candidate)
		return nil, errors.Join(fmt.Errorf("prepare restore readers: %w", err), cleanupErr)
	}
	for _, p := range candidate.panes {
		p.terminal.SetPaletteBase(m.paletteBase)
	}

	m.model = candidate.model
	m.paneMetrics = candidate.paneMetrics
	m.bounds = candidate.bounds
	m.bootstrapped = true
	m.pending = nil
	candidate.committed = true

	events := make([]Event, 0, len(candidate.panes)*2+4)
	for _, p := range candidate.panes {
		address := Event{Window: candidate.paneWindows[p.id], Tab: candidate.paneTabs[p.id], Pane: p.id}
		address.Workspace, _ = m.WorkspaceForWindow(address.Window)
		started, geometry := address, address
		started.Kind = PaneStarted
		geometry.Kind, geometry.Geometry = PaneGeometryChanged, p.geometry
		events = append(events, started, geometry)
	}
	workspace := m.model.ActiveWorkspace()
	window := m.model.activeWindow
	tab := m.model.TabID()
	pane := m.model.FocusedPane()
	events = append(events,
		Event{Kind: WorkspaceActivated, Workspace: workspace.ID},
		Event{Kind: WindowActivated, Workspace: workspace.ID, Window: window},
		Event{Kind: TabActivated, Workspace: workspace.ID, Window: window, Tab: tab},
		Event{Kind: PaneFocused, Workspace: workspace.ID, Window: window, Tab: tab, Pane: pane},
	)
	launchReaders()
	return events, nil
}

// RestoreWindowIDs returns the candidate's ordered workspace/window traversal mapping.
func (m *Mux) RestoreWindowIDs(candidate *RestoreCandidate) ([]WindowID, error) {
	if candidate == nil || candidate.owner != m || m.pending != candidate || candidate.aborted || candidate.committed {
		return nil, ErrInvalidRestore
	}
	return append([]WindowID(nil), candidate.windows...), nil
}

// AbortRestore idempotently tears down an unpublished restore transaction.
func (m *Mux) AbortRestore(candidate *RestoreCandidate) error {
	if candidate == nil || candidate.owner != m {
		return ErrInvalidRestore
	}
	if candidate.committed {
		return ErrRestoreCommitted
	}
	if candidate.aborted {
		return nil
	}
	if m.pending != candidate {
		return ErrInvalidRestore
	}
	return m.abortRestore(candidate)
}

func (m *Mux) abortRestore(candidate *RestoreCandidate) error {
	candidate.aborted = true
	if m.pending == candidate {
		m.pending = nil
	}
	var cleanup []error
	for i := len(candidate.panes) - 1; i >= 0; i-- {
		p := candidate.panes[i]
		detached := m.sessions.abort(p.id, p)
		if detached.owned {
			if err := detached.pane.close(); err != nil {
				cleanup = append(cleanup, fmt.Errorf("pane %d close: %w", p.id, err))
			}
		}
		if !detached.owned {
			if err := p.close(); err != nil {
				cleanup = append(cleanup, fmt.Errorf("pane %d close: %w", p.id, err))
			}
		}
	}
	return errors.Join(cleanup...)
}

func (m *Mux) provisionRestore(candidate *RestoreCandidate, specs []SpawnSpec) error {
	for i, p := range candidate.panes {
		p.setFreshLaunch(specs[i])
		if err := m.sessions.reserve(p.id); err != nil {
			return fmt.Errorf("reserve restore pane %d: %w", p.id, err)
		}
		rows, cols := terminalSize(p.geometry)
		session, err := m.sessions.spawn(rows, cols, specs[i].Options)
		if err != nil {
			m.sessions.release(p.id)
			if session != nil {
				if closeErr := session.Close(); closeErr != nil {
					return errors.Join(fmt.Errorf("spawn restore pane %d: %w", p.id, err), fmt.Errorf("pane %d close: %w", p.id, closeErr))
				}
			}
			return fmt.Errorf("spawn restore pane %d: %w", p.id, err)
		}
		p.session = session
		p.state = PaneStateRunning
		p.desiredSize = pty.Size{Rows: rows, Cols: cols}
		p.appliedSize = p.desiredSize
		p.capture()
		if err := m.sessions.register(p); err != nil {
			m.sessions.release(p.id)
			closeErr := p.close()
			return errors.Join(fmt.Errorf("register restore pane %d: %w", p.id, err), closeErr)
		}
	}
	return nil
}

func buildRestoreCandidate(m *Mux, snapshot layoutrestore.Snapshot, geometries []RestoreWindowGeometry) (restoreBuild, error) {
	if len(snapshot.Workspaces) == 0 || snapshot.ActiveWorkspace < 0 || snapshot.ActiveWorkspace >= len(snapshot.Workspaces) {
		return restoreBuild{}, ErrInvalidRestore
	}
	if len(snapshot.Workspaces[snapshot.ActiveWorkspace].Windows) == 0 {
		return restoreBuild{}, ErrInvalidRestore
	}
	windowCount := 0
	for _, workspace := range snapshot.Workspaces {
		windowCount += len(workspace.Windows)
	}
	if windowCount != len(geometries) || windowCount > MaxWindows || len(snapshot.Workspaces) > MaxWorkspaces {
		return restoreBuild{}, ErrInvalidRestore
	}
	for _, geometry := range geometries {
		if err := validateGeometry(geometry.Content, geometry.Metrics); err != nil || geometry.Content.Empty() {
			return restoreBuild{}, ErrInvalidGeometry
		}
	}

	model := &Model{
		allocatedWorkspaces: make(map[WorkspaceID]struct{}), allocatedWindows: make(map[WindowID]struct{}),
		allocated: make(map[PaneID]struct{}), allocatedSplits: make(map[SplitID]struct{}), allocatedTabs: make(map[TabID]struct{}),
		nextWorkspaceID: m.model.nextWorkspaceID, nextWindowID: m.model.nextWindowID, nextTabID: m.model.nextTabID,
		nextPaneID: m.model.nextPaneID, nextSplitID: m.model.nextSplitID,
	}
	candidate := &RestoreCandidate{owner: m, model: model, paneMetrics: make(map[PaneID]CellMetrics), paneWindows: make(map[PaneID]WindowID), paneTabs: make(map[PaneID]TabID)}
	built := false
	defer func() {
		if built {
			return
		}
		for _, pane := range candidate.panes {
			_ = pane.close()
		}
	}()
	var specs []SpawnSpec
	geometryIndex := 0
	for workspaceIndex, sourceWorkspace := range snapshot.Workspaces {
		name, err := normalizeWorkspaceName(sourceWorkspace.Name)
		if err != nil || name != sourceWorkspace.Name || sourceWorkspace.ActiveWindow < -1 || sourceWorkspace.ActiveWindow >= len(sourceWorkspace.Windows) {
			return restoreBuild{}, ErrInvalidRestore
		}
		workspaceID := model.nextWorkspaceID
		model.nextWorkspaceID++
		workspace := workspaceState{id: workspaceID, name: name, revision: 1}
		model.allocatedWorkspaces[workspaceID] = struct{}{}
		if workspaceIndex == snapshot.ActiveWorkspace {
			model.activeWorkspace = workspaceID
		}
		for windowIndex, sourceWindow := range sourceWorkspace.Windows {
			if sourceWindow.ActiveTab < 0 || sourceWindow.ActiveTab >= len(sourceWindow.Tabs) || len(sourceWindow.Tabs) == 0 || len(sourceWindow.Tabs) > MaxTabs {
				return restoreBuild{}, ErrInvalidRestore
			}
			windowID := model.nextWindowID
			model.nextWindowID++
			window := windowState{id: windowID, workspace: workspaceID, title: sourceWindow.Title, revision: 1}
			candidate.windows = append(candidate.windows, windowID)
			model.allocatedWindows[windowID] = struct{}{}
			workspace.windows = append(workspace.windows, windowID)
			if windowIndex == sourceWorkspace.ActiveWindow {
				workspace.active = windowID
			}
			geometry := geometries[geometryIndex]
			geometryIndex++
			for tabIndex, sourceTab := range sourceWindow.Tabs {
				tabID := model.nextTabID
				model.nextTabID++
				root, leaves, leafSpecs, err := buildRestoreNode(model, sourceTab.Root)
				if err != nil || sourceTab.FocusedLeaf < 0 || sourceTab.FocusedLeaf >= len(leaves) {
					return restoreBuild{}, ErrInvalidRestore
				}
				tab := tabState{id: tabID, title: sourceTab.Title, root: root, focused: leaves[sourceTab.FocusedLeaf], revision: 1}
				model.allocatedTabs[tabID] = struct{}{}
				window.tabs = append(window.tabs, tab)
				if tabIndex == sourceWindow.ActiveTab {
					window.active = tabID
				}
				layout, err := layoutRoot(root, geometry.Content, UniformCellMetrics(geometry.Metrics))
				if err != nil || layout.Compressed {
					if err != nil {
						return restoreBuild{}, err
					}
					return restoreBuild{}, ErrSplitTooSmall
				}
				for _, paneGeometry := range layout.Panes {
					p := m.createPane(paneGeometry.Pane, paneGeometry.Cols, paneGeometry.Rows)
					p.terminal.SetPaletteBase(m.paletteBase)
					p.geometry = paneGeometry
					if m.options.SetClipboard != nil {
						p.parser.SetClipboard = func(text string) { m.options.SetClipboard(p.id, text) }
					}
					candidate.panes = append(candidate.panes, p)
					candidate.paneMetrics[p.id] = geometry.Metrics
					candidate.paneWindows[p.id] = windowID
					candidate.paneTabs[p.id] = tabID
				}
				specs = append(specs, leafSpecs...)
			}
			model.windows = append(model.windows, window)
			if workspaceIndex == snapshot.ActiveWorkspace && windowIndex == sourceWorkspace.ActiveWindow {
				model.activeWindow = windowID
				candidate.bounds = geometry.Content
			}
		}
		model.workspaces = append(model.workspaces, workspace)
	}
	if err := model.CheckInvariants(); err != nil {
		return restoreBuild{}, err
	}
	built = true
	return restoreBuild{candidate: candidate, specs: specs}, nil
}

func buildRestoreNode(model *Model, source layoutrestore.Node) (*node, []PaneID, []SpawnSpec, error) {
	switch source.Type {
	case "pane":
		if source.Launch == nil || source.First != nil || source.Second != nil || source.Axis != "" || source.Ratio != 0 {
			return nil, nil, nil, ErrInvalidRestore
		}
		paneID := model.nextPaneID
		model.nextPaneID++
		model.allocated[paneID] = struct{}{}
		spec := SpawnSpec{TargetID: source.Launch.TargetID, Options: pty.Options{ShellProgram: source.Launch.Program, ShellArgs: append([]string(nil), source.Launch.Args...), WorkingDirectory: source.Launch.CWD}}
		return leafNode(paneID), []PaneID{paneID}, []SpawnSpec{spec}, nil
	case "split":
		if source.Launch != nil || source.First == nil || source.Second == nil {
			return nil, nil, nil, ErrInvalidRestore
		}
		axis := SplitColumns
		if source.Axis == "rows" {
			axis = SplitRows
		} else if source.Axis != "columns" {
			return nil, nil, nil, ErrInvalidRestore
		}
		ratio := SplitRatio(source.Ratio)
		if !validRatio(ratio) {
			return nil, nil, nil, ErrInvalidRestore
		}
		first, firstLeaves, firstSpecs, err := buildRestoreNode(model, *source.First)
		if err != nil {
			return nil, nil, nil, err
		}
		second, secondLeaves, secondSpecs, err := buildRestoreNode(model, *source.Second)
		if err != nil {
			return nil, nil, nil, err
		}
		splitID := model.nextSplitID
		model.nextSplitID++
		model.allocatedSplits[splitID] = struct{}{}
		return branchNode(splitID, axis, ratio, first, second), append(firstLeaves, secondLeaves...), append(firstSpecs, secondSpecs...), nil
	default:
		return nil, nil, nil, ErrInvalidRestore
	}
}

func (m *Mux) restorePanePending(id PaneID) bool {
	if m.pending == nil {
		return false
	}
	_, ok := m.pending.paneMetrics[id]
	return ok
}
