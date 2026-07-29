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
	ownerStamp  ownerStamp
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

type muxRestorePreparationOperationAdapter struct {
	mux        *Mux
	blueprint  layoutrestore.Blueprint
	geometries []RestoreWindowGeometry
	scope      mutationScope
}

type muxRestorePublicationOperationAdapter struct {
	mux   *Mux
	scope mutationScope
}

// PrepareRestore validates and provisions a detached startup restore transaction.
func (m *Mux) prepareRestore(scope mutationScope, blueprint layoutrestore.Blueprint, geometries []RestoreWindowGeometry) (*RestoreCandidate, error) {
	if err := scope.valid(m); err != nil {
		return nil, err
	}
	return m.restoreCoordinator.prepareRestore(muxRestorePreparationOperationAdapter{mux: m, blueprint: blueprint, geometries: geometries, scope: scope})
}

func (a muxRestorePreparationOperationAdapter) prepareRestore() (*RestoreCandidate, error) {
	m := a.mux
	if err := a.scope.valid(m); err != nil {
		return nil, err
	}
	if err := m.retryUnpublishedRollback(a.scope); err != nil {
		return nil, fmt.Errorf("retry unpublished restore rollback: %w", err)
	}
	blueprint := a.blueprint
	geometries := a.geometries
	if m.pending != nil {
		return nil, ErrRestorePending
	}
	panes, reserved, started := m.sessions.activeCounts()
	if m.bootstrapped || panes != 0 || reserved != 0 || started != 0 {
		return nil, ErrRestorePrecondition
	}

	build, err := buildRestoreCandidateScoped(a.scope, m, blueprint.Snapshot(), geometries)
	if err != nil {
		return nil, err
	}
	candidate := build.candidate
	rollback := newUnpublishedPaneRollback(candidate.panes)
	preparedCloses, err := retainPaneClosuresWithFault(candidate.panes, func(p *pane) error {
		return m.transactionFailure("restore-close-preflight", p)
	})
	rollback.retainPrepared(preparedCloses)
	if err != nil {
		candidate.aborted = true
		return nil, errors.Join(err, m.rollbackUnpublishedPanes(a.scope, rollback))
	}
	m.pending = candidate
	if err := m.provisionRestore(a.scope, candidate, build.specs, rollback); err != nil {
		cleanupErr := m.abortRestorePreparation(a.scope, candidate, rollback)
		return nil, errors.Join(err, cleanupErr)
	}
	if abortErr := abortPaneClosuresWithFault(preparedCloses, func(prepared *preparedPaneClose) error {
		return m.transactionFailure("restore-close-abort", prepared.pane)
	}); abortErr != nil {
		cleanupErr := m.abortRestorePreparation(a.scope, candidate, rollback)
		return nil, errors.Join(abortErr, cleanupErr)
	}
	return candidate, nil
}

// CommitRestore atomically publishes the exact pending restore transaction.
func (m *Mux) commitRestore(scope mutationScope, candidate *RestoreCandidate) ([]Event, error) {
	if err := scope.valid(m); err != nil {
		return nil, err
	}
	return m.restoreCoordinator.commitRestore(candidate, muxRestorePublicationOperationAdapter{mux: m, scope: scope})
}

func (a muxRestorePublicationOperationAdapter) commitRestore(candidate *RestoreCandidate) ([]Event, error) {
	m := a.mux
	if err := a.scope.valid(m); err != nil {
		return nil, err
	}
	if candidate != nil {
		if err := candidate.ownerStamp.validPrepared(m); err != nil {
			return nil, err
		}
	}
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
	launchReaders, err := m.sessions.prepareStartsScoped(a.scope, ids)
	if err != nil {
		cleanupErr := m.abortRestore(a.scope, candidate)
		return nil, errors.Join(fmt.Errorf("prepare restore readers: %w", err), cleanupErr)
	}
	for _, p := range candidate.panes {
		p.terminal.SetPaletteBase(*m.paletteBase)
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
	return m.restoreCoordinator.restoreWindowIDs(candidate, muxRestorePublicationOperationAdapter{mux: m})
}

func (a muxRestorePublicationOperationAdapter) restoreWindowIDs(candidate *RestoreCandidate) ([]WindowID, error) {
	m := a.mux
	if candidate != nil {
		if err := candidate.ownerStamp.validPrepared(m); err != nil {
			return nil, err
		}
	}
	if candidate == nil || candidate.owner != m || m.pending != candidate || candidate.aborted || candidate.committed {
		return nil, ErrInvalidRestore
	}
	return append([]WindowID(nil), candidate.windows...), nil
}

// AbortRestore idempotently tears down an unpublished restore transaction.
func (m *Mux) abortRestoreOwned(scope mutationScope, candidate *RestoreCandidate) error {
	if err := scope.valid(m); err != nil {
		return err
	}
	return m.restoreCoordinator.abortRestore(candidate, muxRestorePublicationOperationAdapter{mux: m, scope: scope})
}

func (a muxRestorePublicationOperationAdapter) abortRestore(candidate *RestoreCandidate) error {
	m := a.mux
	if err := a.scope.valid(m); err != nil {
		return err
	}
	if candidate != nil {
		if err := candidate.ownerStamp.validPrepared(m); err != nil {
			return err
		}
	}
	if candidate == nil || candidate.owner != m {
		return ErrInvalidRestore
	}
	if candidate.committed {
		return ErrRestoreCommitted
	}
	if candidate.aborted {
		return m.retryUnpublishedRollback(a.scope)
	}
	if m.pending != candidate {
		return ErrInvalidRestore
	}
	return m.abortRestore(a.scope, candidate)
}

func (m *Mux) abortRestore(scope mutationScope, candidate *RestoreCandidate) error {
	if err := scope.valid(m); err != nil {
		return err
	}
	preparedCloses, err := preparePaneClosures(candidate.panes)
	if err != nil {
		return err
	}
	return m.abortRestorePrepared(scope, candidate, preparedCloses)
}

func (m *Mux) abortRestorePrepared(scope mutationScope, candidate *RestoreCandidate, preparedCloses []*preparedPaneClose) error {
	if err := scope.valid(m); err != nil {
		return err
	}
	rollback := newUnpublishedPaneRollback(candidate.panes)
	rollback.retainPrepared(preparedCloses)
	for index := range rollback.registered {
		rollback.registered[index] = true
	}
	return m.abortRestorePreparation(scope, candidate, rollback)
}

func (m *Mux) abortRestorePreparation(scope mutationScope, candidate *RestoreCandidate, rollback *unpublishedPaneRollback) error {
	if err := scope.valid(m); err != nil {
		return err
	}
	candidate.aborted = true
	if m.pending == candidate {
		m.pending = nil
	}
	return m.rollbackUnpublishedPanes(scope, rollback)
}

func (m *Mux) provisionRestore(scope mutationScope, candidate *RestoreCandidate, specs []SpawnSpec, rollback *unpublishedPaneRollback) error {
	if err := scope.valid(m); err != nil {
		return err
	}
	for i, p := range candidate.panes {
		p.setFreshLaunch(specs[i])
		if err := m.sessions.reserveScoped(scope, p.id); err != nil {
			return fmt.Errorf("reserve restore pane %d: %w", p.id, err)
		}
		rows, cols := terminalSize(p.geometry)
		session, err := m.sessions.spawnScoped(scope, rows, cols, specs[i].Options)
		p.session = session
		if err != nil {
			m.sessions.releaseScoped(scope, p.id)
			return fmt.Errorf("spawn restore pane %d: %w", p.id, err)
		}
		p.state = PaneStateRunning
		p.desiredSize = pty.Size{Rows: rows, Cols: cols}
		p.appliedSize = p.desiredSize
		p.capture()
		if err := m.sessions.registerScoped(scope, p); err != nil {
			return fmt.Errorf("register restore pane %d: %w", p.id, err)
		}
		rollback.markRegistered(p)
	}
	return nil
}

func buildRestoreCandidateScoped(scope mutationScope, m *Mux, snapshot layoutrestore.Snapshot, geometries []RestoreWindowGeometry) (result restoreBuild, resultErr error) {
	if err := scope.valid(m); err != nil {
		return restoreBuild{}, err
	}
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
		nextWorkspaceID: m.model.nextWorkspaceID, nextWindowID: m.model.nextWindowID, nextWindowIncarnation: m.model.nextWindowIncarnation, nextTabID: m.model.nextTabID,
		nextPaneID: m.model.nextPaneID, nextSplitID: m.model.nextSplitID,
	}
	candidate := &RestoreCandidate{owner: m, ownerStamp: m.currentOwnerStamp(), model: model, paneMetrics: make(map[PaneID]CellMetrics), paneWindows: make(map[PaneID]WindowID), paneTabs: make(map[PaneID]TabID)}
	built := false
	defer func() {
		if built {
			return
		}
		candidate.aborted = true
		resultErr = errors.Join(resultErr, m.rollbackUnpublishedPanes(scope, newUnpublishedPaneRollback(candidate.panes)))
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
			incarnation := model.nextWindowIncarnation
			if incarnation == 0 || incarnation == ^WindowIncarnation(0) {
				return restoreBuild{}, ErrIDExhausted
			}
			model.nextWindowID++
			model.nextWindowIncarnation++
			window := windowState{id: windowID, incarnation: incarnation, workspace: workspaceID, title: sourceWindow.Title, revision: 1}
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
					p := m.createPane(scope, paneGeometry.Pane, paneGeometry.Cols, paneGeometry.Rows)
					p.terminal.SetPaletteBase(*m.paletteBase)
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
