package mux

import (
	"strings"
	"testing"
)

func TestOrderedTabFoundationInitialCompatibility(t *testing.T) {
	m := NewModel()
	if len(m.tabs) != 1 || m.active != 1 || m.TabID() != 1 || m.FocusedPane() != 1 {
		t.Fatalf("model=%#v", m)
	}
	if ids := m.PaneIDs(); len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("panes=%v", ids)
	}
	if err := m.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestOrderedTabFoundationTracksIndependentRootsAndRememberedFocus(t *testing.T) {
	m := NewModel()
	m.tabs = append(m.tabs, tabState{id: 2, root: leafNode(2), focused: 2})
	m.allocatedTabs[2] = struct{}{}
	m.allocated[2] = struct{}{}
	m.nextTabID = 3
	m.nextPaneID = 3
	if err := m.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
	m.active = 2
	if m.FocusedPane() != 2 || m.TabID() != 2 {
		t.Fatalf("active=%d focus=%d", m.TabID(), m.FocusedPane())
	}
	m.active = 1
	if m.FocusedPane() != 1 {
		t.Fatalf("remembered focus=%d", m.FocusedPane())
	}
}

func TestOrderedTabFoundationRejectsDuplicateOwnership(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Model)
		want   string
	}{
		{"duplicate tab", func(m *Model) {
			m.tabs = append(m.tabs, tabState{id: 1, root: leafNode(2), focused: 2})
			m.allocated[2] = struct{}{}
			m.nextPaneID = 3
		}, "appears more than once"},
		{"shared pane", func(m *Model) {
			m.tabs = append(m.tabs, tabState{id: 2, root: leafNode(1), focused: 1})
			m.allocatedTabs[2] = struct{}{}
			m.nextTabID = 3
		}, "belongs to tabs"},
		{"missing active", func(m *Model) { m.active = 9 }, "expected one active"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel()
			tc.mutate(m)
			if err := m.CheckInvariants(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
		})
	}
}

func TestOrderedTabFoundationEnforcesBound(t *testing.T) {
	m := NewModel()
	for id := TabID(2); id <= MaxTabs+1; id++ {
		pane := PaneID(id)
		m.tabs = append(m.tabs, tabState{id: id, root: leafNode(pane), focused: pane})
		m.allocatedTabs[id] = struct{}{}
		m.allocated[pane] = struct{}{}
	}
	m.nextTabID = MaxTabs + 2
	m.nextPaneID = PaneID(MaxTabs + 2)
	if err := m.CheckInvariants(); err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("err=%v", err)
	}
}
