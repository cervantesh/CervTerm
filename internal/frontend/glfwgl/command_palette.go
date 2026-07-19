//go:build glfw

package glfwgl

import (
	"fmt"
	"sort"

	termaction "cervterm/internal/action"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"
)

const maxCommandPaletteEntries = 256

type commandPaletteActivation struct {
	envelope   termaction.Envelope
	callback   *script.CallbackRef
	generation uint64
}
type commandPaletteItem struct {
	entry      modal.Entry
	activation commandPaletteActivation
}

func (a *App) openCommandPalette() error {
	pane, ok := a.mux.FocusedPane()
	if !ok {
		return termaction.ErrTargetUnavailable
	}
	entries, actions := a.commandPaletteSnapshot(true)
	if len(entries) == 0 || !a.modal.Open(modal.ModeCommandPalette, modal.PaneIdentity(pane), modal.FocusIdentity(pane), entries) {
		return fmt.Errorf("command palette has no available commands")
	}
	if a.search.active {
		a.search.close()
	}
	a.commandPalette = actions
	a.requestRedraw()
	return nil
}

func (a *App) commandPaletteSnapshot(hasPane bool) ([]modal.Entry, map[string]commandPaletteActivation) {
	items := make([]commandPaletteItem, 0, maxCommandPaletteEntries)
	add := func(id, label, detail, category string, envelope termaction.Envelope, callback *script.CallbackRef) {
		if label == "" || len(items) >= maxCommandPaletteEntries {
			return
		}
		items = append(items, commandPaletteItem{modal.Entry{ID: id, Label: label, Detail: detail, Category: category}, commandPaletteActivation{envelope: envelope, callback: callback, generation: a.scriptGeneration}})
	}
	for _, d := range termaction.DefaultRegistry().Descriptors() {
		if !d.Discoverable || (d.Target == termaction.TargetPane && !hasPane) {
			continue
		}
		var command termaction.Action
		switch d.ID {
		case termaction.IDCopySelection:
			command = termaction.CopySelection{}
		case termaction.IDPasteClipboard:
			command = termaction.PasteClipboard{}
		case termaction.IDToggleSearch:
			command = termaction.ToggleSearch{}
		case termaction.IDToggleStats:
			command = termaction.ToggleStats{}
		case termaction.IDReloadConfig:
			command = termaction.ReloadConfig{}
		case termaction.IDClosePane:
			command = termaction.ClosePane{}
		case termaction.IDActivateCommandPalette:
			command = termaction.ActivateCommandPalette{}
		case termaction.IDActivateQuickSelect:
			command = termaction.ActivateQuickSelect{}
		case termaction.IDActivateLaunchMenu:
			command = termaction.ActivateLaunchMenu{}
		case termaction.IDNewTab:
			command = termaction.NewTab{}
		case termaction.IDNewWindow:
			command = termaction.NewWindow{}
		case termaction.IDCloseWindow:
			if a.windowID == 0 {
				continue
			}
			command = termaction.CloseWindow{WindowID: uint64(a.windowID)}
		case termaction.IDFocusWindow:
			if a.windowID == 0 {
				continue
			}
			command = termaction.FocusWindow{WindowID: uint64(a.windowID)}
		case termaction.IDActivateTabRelative:
			command = termaction.ActivateTabRelative{Delta: 1}
		case termaction.IDActivateTabSwitcher:
			command = termaction.ActivateTabSwitcher{}
		case termaction.IDActivateWorkspaceSwitcher:
			command = termaction.ActivateWorkspaceSwitcher{}
		default:
			continue
		}
		add("action:"+string(d.ID), d.Label, "", string(d.Category), actionEnvelope(command), nil)
	}
	if a.scriptRT != nil {
		set := a.scriptRT.BindingSet()
		for i, b := range set.Root {
			a.addPaletteBinding(&items, "root", "", i, b.Spec.String(), b.Label, b.Action, b.Callback)
		}
		for _, table := range set.Tables {
			for i, b := range table.Bindings {
				a.addPaletteBinding(&items, "table", table.Name, i, b.Spec.String(), b.Label, b.Action, b.Callback)
			}
		}
		for i, b := range set.Mouse {
			a.addPaletteBinding(&items, "mouse", "", i, fmt.Sprintf("%s %s", b.Spec.Event, b.Spec.Button), b.Label, b.Action, b.Callback)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].entry.Label != items[j].entry.Label {
			return items[i].entry.Label < items[j].entry.Label
		}
		return items[i].entry.ID < items[j].entry.ID
	})
	entries := make([]modal.Entry, len(items))
	activations := make(map[string]commandPaletteActivation, len(items))
	for i := range items {
		entries[i] = items[i].entry
		activations[items[i].entry.ID] = items[i].activation
	}
	return entries, activations
}

func (a *App) addPaletteBinding(items *[]commandPaletteItem, domain, table string, slot int, shortcut, label string, envelope termaction.Envelope, callback *script.CallbackRef) {
	if label == "" || len(*items) >= maxCommandPaletteEntries {
		return
	}
	d, err := termaction.DefaultRegistry().Describe(envelope.Action)
	if err != nil || !d.Discoverable {
		return
	}
	if d.Target == termaction.TargetPane {
		if _, ok := a.mux.FocusedPane(); !ok {
			return
		}
	}
	id := fmt.Sprintf("binding:%s/%s/%d", domain, table, slot)
	var ref *script.CallbackRef
	if callback != nil {
		copy := *callback
		ref = &copy
	}
	*items = append(*items, commandPaletteItem{modal.Entry{ID: id, Label: label, Detail: shortcut, Category: domain}, commandPaletteActivation{envelope: envelope, callback: ref, generation: a.scriptGeneration}})
}

func (a *App) acceptCommandPalette(entry modal.Entry, pane termmux.PaneID) error {
	activation, ok := a.commandPalette[entry.ID]
	if !ok {
		return fmt.Errorf("command is no longer available")
	}
	if activation.callback != nil {
		if a.scriptRT == nil || activation.generation != a.scriptGeneration {
			return fmt.Errorf("command callback is unavailable after reload")
		}
		return a.scriptRT.DispatchRef(*activation.callback, entry.Label, paneHost{app: a, pane: pane})
	}
	ctx := a.actionContext(termaction.SourceKeyboard)
	ctx.Origin = termaction.Ref{Kind: termaction.RefPane, ID: uint64(pane)}
	return a.executeAction(activation.envelope, ctx)
}
