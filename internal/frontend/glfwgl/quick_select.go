//go:build glfw

package glfwgl

import (
	"cervterm/internal/linkpolicy"
	"fmt"
	"strings"

	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"cervterm/internal/quickselect"
)

type quickSelectActivation struct {
	snapshot       termmux.QuickSelectSnapshot
	candidates     map[string]quickselect.Candidate
	userActivation bool
	setClipboard   func(string)
}

func (a *App) openQuickSelect(pane termmux.PaneID) error {
	if _, ok := a.mux.PaneView(pane); !ok {
		return fmt.Errorf("quick select requires an available pane")
	}
	snapshot, ok := a.mux.QuickSelectSnapshot(pane, 0, 0)
	if !ok {
		return fmt.Errorf("quick select snapshot is unavailable")
	}
	candidates := quickselect.Find(snapshot, a.cfg.QuickSelect.Compiled)
	if len(candidates) == 0 {
		return fmt.Errorf("quick select found no candidates")
	}
	entries := make([]modal.Entry, len(candidates))
	byLabel := make(map[string]quickselect.Candidate, len(candidates))
	for i, candidate := range candidates {
		entries[i] = modal.Entry{ID: candidate.Label, Label: candidate.Label, Detail: candidate.Text}
		byLabel[candidate.Label] = candidate
	}
	if !a.modal.Open(modal.ModeQuickSelect, modal.PaneIdentity(pane), modal.FocusIdentity(pane), entries) {
		return fmt.Errorf("quick select could not open")
	}
	a.quickSelect = quickSelectActivation{snapshot: snapshot, candidates: byLabel, setClipboard: a.quickSelect.setClipboard}
	a.requestRedraw()
	return nil
}

func (a *App) acceptQuickSelect(entry modal.Entry, pane termmux.PaneID) error {
	activation := a.quickSelect
	explicitActivation := a.quickSelect.userActivation
	a.quickSelect.userActivation = false
	candidate, ok := activation.candidates[entry.ID]
	if !ok || pane != activation.snapshot.PaneID {
		return fmt.Errorf("quick select candidate is unavailable")
	}
	// This check is intentionally immediately adjacent to the side effect.
	if !a.mux.QuickSelectSnapshotCurrent(activation.snapshot) {
		return fmt.Errorf("quick select candidate is stale")
	}
	switch candidate.Action {
	case quickselect.ActionCopy:
		if a.quickSelect.setClipboard != nil {
			a.quickSelect.setClipboard(candidate.Text)
		} else {
			a.SetClipboard(candidate.Text)
		}
	case quickselect.ActionOpen:
		decision := linkpolicy.Evaluate(candidate.Text, linkpolicy.Activation{Explicit: explicitActivation, Fresh: true})
		if !decision.Allowed() {
			return fmt.Errorf("quick select URL must be absolute http or https (%s)", decision.Denial)
		}
		if a.linkLauncher == nil {
			return fmt.Errorf("link launcher unavailable")
		}
		if err := a.linkLauncher.Launch(decision.URI); err != nil {
			return fmt.Errorf("no se pudo abrir %s", decision.SafeLabel)
		}
	default:
		return fmt.Errorf("quick select action %q is invalid", candidate.Action)
	}
	return nil
}

func (a *App) handleQuickSelectChar(char rune) bool {
	if a.modal.Mode() != modal.ModeQuickSelect {
		return false
	}
	a.modal.AppendRune(char)
	state := a.modal.Snapshot()
	prefix := strings.ToLower(string(state.Query))
	matchedPrefix := false
	for _, entry := range state.Entries {
		label := strings.ToLower(entry.ID)
		if label == prefix {
			a.quickSelect.userActivation = true
			a.applyModalIntents([]modal.Intent{{Kind: modal.IntentAccept, Entry: entry, Pane: state.OpeningPane}})
			return true
		}
		matchedPrefix = matchedPrefix || strings.HasPrefix(label, prefix)
	}
	if !matchedPrefix {
		a.modal.SetError("quick select label does not match")
	}
	a.requestRedraw()
	return true
}
