//go:build glfw

package glfwgl

import (
	"cervterm/internal/layoutstate"
	termmux "cervterm/internal/mux"
)

func (a *App) persistCurrentLayout() error {
	if a == nil || !a.cfg.LayoutPersistence.Enabled || a.controller == nil {
		return nil
	}
	plan, err := a.controller.currentLayoutPlan()
	if err != nil {
		return err
	}
	store, err := layoutstate.NewStore(layoutstate.StoreOptions{Path: a.cfg.LayoutPersistence.Path})
	if err != nil {
		return err
	}
	return store.Save(plan)
}

func layoutPersistenceEvent(events []termmux.Event) bool {
	for _, event := range events {
		switch event.Kind {
		case termmux.PaneStarted, termmux.PaneFocused, termmux.PaneClosed, termmux.PaneTransferred,
			termmux.TabSpawned, termmux.TabActivated, termmux.TabRenamed, termmux.TabMoved, termmux.TabRevisionChanged, termmux.TabClosed,
			termmux.WindowCreated, termmux.WindowActivated, termmux.WindowRenamed, termmux.WorkspaceCreated, termmux.WorkspaceActivated,
			termmux.WorkspaceRenamed, termmux.WindowWorkspaceChanged:
			return true
		}
	}
	return false
}
