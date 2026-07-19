//go:build glfw

package glfwgl

import (
	"fmt"

	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"cervterm/internal/pty"
)

func (a *App) openLaunchMenu(pane termmux.PaneID) error {
	if _, ok := a.mux.PaneView(pane); !ok {
		return fmt.Errorf("launch menu requires an available pane")
	}
	targets := a.desiredCfg.LaunchMenu
	if len(targets) == 0 {
		return fmt.Errorf("launch menu has no configured targets")
	}
	entries := make([]modal.Entry, len(targets))
	for i, target := range targets {
		entries[i] = modal.Entry{ID: target.ID, Label: target.Label, Detail: target.Program, Category: "launch"}
	}
	if !a.modal.Open(modal.ModeLaunchMenu, modal.PaneIdentity(pane), modal.FocusIdentity(pane), entries) {
		return fmt.Errorf("launch menu could not open")
	}
	a.requestRedraw()
	return nil
}

func (a *App) acceptLaunchMenu(entry modal.Entry, pane termmux.PaneID) error {
	var selected *configLaunchTarget
	for i := range a.desiredCfg.LaunchMenu {
		target := &a.desiredCfg.LaunchMenu[i]
		if target.ID == entry.ID {
			selected = &configLaunchTarget{id: target.ID, program: target.Program, args: append([]string(nil), target.Args...), cwd: target.CWD, env: cloneLaunchEnvironment(target.Env)}
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("launch target %q is no longer available", entry.ID)
	}
	_, events, err := a.mux.SpawnSplit(pane, termmux.SplitColumns, termmux.SpawnSpec{TargetID: selected.id, Options: pty.Options{ShellProgram: selected.program, ShellArgs: selected.args, WorkingDirectory: selected.cwd, Env: selected.env}})
	if err != nil {
		return err
	}
	a.handleMuxEvents(events)
	a.syncFocusedProjection()
	a.requestRedraw()
	return nil
}

type configLaunchTarget struct {
	id, program string
	args        []string
	cwd         string
	env         map[string]string
}

func cloneLaunchEnvironment(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	out := make(map[string]string, len(source))
	for k, v := range source {
		out[k] = v
	}
	return out
}
