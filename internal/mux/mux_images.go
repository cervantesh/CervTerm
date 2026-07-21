package mux

import "cervterm/internal/termimage"

func (m *Mux) createPane(id PaneID, cols, rows int) *pane {
	pane := newPane(id, cols, rows, m.options.ScrollbackCapacity, m.options.HideCursorWhenScrolled)
	if m.imageBudget == nil {
		return pane
	}
	store := termimage.NewStore(m.imageBudget, m.imageLimits)
	if store == nil {
		return pane
	}
	if err := pane.terminal.AttachImageStore(store); err != nil {
		store.Close()
		return pane
	}
	pane.imageStore = store
	pane.capture()
	return pane
}

// AcquireImageResource returns a detached exact-generation resource for a published pane.
func (m *Mux) AcquireImageResource(id PaneID, ref termimage.ResourceRef) (termimage.DetachedResource, bool) {
	if m == nil || id == 0 || ref.Image == 0 || ref.Generation == 0 || m.model.tabForPane(id) == nil {
		return termimage.DetachedResource{}, false
	}
	pane, ok := m.sessions.lookup(id)
	if !ok || pane.imageStore == nil {
		return termimage.DetachedResource{}, false
	}
	return pane.imageStore.Acquire(ref)
}

// ImageSetupError reports invalid programmatic image limits without changing legacy startup behavior.
func (m *Mux) ImageSetupError() error {
	if m == nil {
		return nil
	}
	return m.imageSetupErr
}
