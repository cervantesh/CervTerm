package mux

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxWorkspaces         = 64
	MaxWorkspaceNameBytes = 128
	DefaultWorkspaceName  = "default"
)

// WorkspaceID is a stable, process-local workspace identity. Zero is invalid.
type WorkspaceID uint64

type workspaceState struct {
	id       WorkspaceID
	name     string
	windows  []WindowID
	active   WindowID
	revision uint64
}

// WorkspaceView is a detached immutable projection of workspace membership.
type WorkspaceView struct {
	ID       WorkspaceID
	Name     string
	Windows  []WindowID
	Active   bool
	Focused  WindowID
	Revision uint64
}

func normalizeWorkspaceName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", ErrInvalidWorkspaceName
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrInvalidWorkspaceName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrInvalidWorkspaceName
		}
	}
	if len(name) > MaxWorkspaceNameBytes {
		return "", ErrWorkspaceNameTooLong
	}
	return name, nil
}

func (m *Model) workspaceByID(id WorkspaceID) *workspaceState {
	for i := range m.workspaces {
		if m.workspaces[i].id == id {
			return &m.workspaces[i]
		}
	}
	return nil
}

func (m *Model) workspaceForWindow(id WindowID) *workspaceState {
	w := m.windowByID(id)
	if w == nil {
		return nil
	}
	return m.workspaceByID(w.workspace)
}

func (m *Model) Workspaces() []WorkspaceView {
	out := make([]WorkspaceView, len(m.workspaces))
	for i := range m.workspaces {
		ws := &m.workspaces[i]
		out[i] = WorkspaceView{ID: ws.id, Name: ws.name, Windows: append([]WindowID(nil), ws.windows...), Active: ws.id == m.activeWorkspace, Focused: ws.active, Revision: ws.revision}
	}
	return out
}

func (m *Model) ActiveWorkspace() WorkspaceView {
	ws := m.workspaceByID(m.activeWorkspace)
	if ws == nil {
		return WorkspaceView{}
	}
	return WorkspaceView{ID: ws.id, Name: ws.name, Windows: append([]WindowID(nil), ws.windows...), Active: true, Focused: ws.active, Revision: ws.revision}
}

func (m *Model) CreateWorkspace(name string) (WorkspaceView, error) {
	name, err := normalizeWorkspaceName(name)
	if err != nil {
		return WorkspaceView{}, err
	}
	if len(m.workspaces) >= MaxWorkspaces {
		return WorkspaceView{}, ErrWorkspaceLimitReached
	}
	if m.nextWorkspaceID == 0 {
		return WorkspaceView{}, ErrIDExhausted
	}
	for i := range m.workspaces {
		if m.workspaces[i].name == name {
			return WorkspaceView{}, ErrWorkspaceNameExists
		}
	}
	ws := workspaceState{id: m.nextWorkspaceID, name: name, revision: 1}
	m.workspaces = append(m.workspaces, ws)
	m.allocatedWorkspaces[ws.id] = struct{}{}
	m.nextWorkspaceID++
	if err := m.CheckInvariants(); err != nil {
		m.workspaces = m.workspaces[:len(m.workspaces)-1]
		delete(m.allocatedWorkspaces, ws.id)
		m.nextWorkspaceID = ws.id
		return WorkspaceView{}, err
	}
	return WorkspaceView{ID: ws.id, Name: ws.name, Revision: 1}, nil
}

func (m *Model) RenameWorkspace(id WorkspaceID, name string) error {
	name, err := normalizeWorkspaceName(name)
	if err != nil {
		return err
	}
	ws := m.workspaceByID(id)
	if ws == nil {
		return ErrWorkspaceNotFound
	}
	for i := range m.workspaces {
		if m.workspaces[i].id != id && m.workspaces[i].name == name {
			return ErrWorkspaceNameExists
		}
	}
	if ws.name == name {
		return nil
	}
	previousName, previousRevision := ws.name, ws.revision
	ws.name = name
	ws.revision++
	if err := m.CheckInvariants(); err != nil {
		ws.name, ws.revision = previousName, previousRevision
		return err
	}
	return nil
}

func (m *Model) SwitchWorkspace(id WorkspaceID) error {
	ws := m.workspaceByID(id)
	if ws == nil {
		return ErrWorkspaceNotFound
	}
	previousWorkspaces, previousWorkspace, previousWindow := cloneWorkspaceStates(m.workspaces), m.activeWorkspace, m.activeWindow
	if m.activeWorkspace == id {
		return nil
	}
	if source := m.workspaceByID(m.activeWorkspace); source != nil {
		source.revision++
	}
	ws.revision++
	m.activeWorkspace = id
	m.activeWindow = ws.active
	if err := m.CheckInvariants(); err != nil {
		m.workspaces, m.activeWorkspace, m.activeWindow = previousWorkspaces, previousWorkspace, previousWindow
		return err
	}
	return nil
}

func (m *Model) MoveWindowToWorkspace(window WindowID, target WorkspaceID) error {
	w := m.windowByID(window)
	to := m.workspaceByID(target)
	if w == nil {
		return ErrWindowNotFound
	}
	if to == nil {
		return ErrWorkspaceNotFound
	}
	previousWindows, previousWorkspaces, previousActive := cloneWindowStates(m.windows), cloneWorkspaceStates(m.workspaces), m.activeWindow
	if w.workspace == target {
		return nil
	}
	from := m.workspaceByID(w.workspace)
	if from == nil {
		return invariantError("window %d has missing workspace %d", window, w.workspace)
	}
	fromIndex := -1
	for i, id := range from.windows {
		if id == window {
			fromIndex = i
			break
		}
	}
	if fromIndex < 0 {
		return invariantError("workspace %d does not own window %d", from.id, window)
	}
	from.windows = append(from.windows[:fromIndex], from.windows[fromIndex+1:]...)
	if from.active == window {
		from.active = 0
		if len(from.windows) > 0 {
			if fromIndex >= len(from.windows) {
				fromIndex = len(from.windows) - 1
			}
			from.active = from.windows[fromIndex]
		}
	}
	to.windows = append(to.windows, window)
	if to.active == 0 {
		to.active = window
	}
	w.workspace = target
	w.revision++
	from.revision++
	to.revision++
	if m.activeWorkspace == from.id {
		m.activeWindow = from.active
	}
	if m.activeWorkspace == to.id {
		m.activeWindow = to.active
	}
	if err := m.CheckInvariants(); err != nil {
		m.windows, m.workspaces, m.activeWindow = previousWindows, previousWorkspaces, previousActive
		return err
	}
	return nil
}

func cloneWorkspaceStates(in []workspaceState) []workspaceState {
	out := make([]workspaceState, len(in))
	copy(out, in)
	for i := range out {
		out[i].windows = append([]WindowID(nil), in[i].windows...)
	}
	return out
}

func (m *Model) reconcileActiveWorkspaceWindow() {
	ws := m.workspaceByID(m.activeWorkspace)
	if ws == nil {
		return
	}
	if active := m.windowByID(m.activeWindow); active != nil && active.workspace == ws.id {
		if ws.active != active.id {
			ws.active = active.id
			ws.revision++
		}
		return
	}
	m.activeWindow = ws.active
}
