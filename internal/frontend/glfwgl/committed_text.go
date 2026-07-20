//go:build glfw

package glfwgl

import (
	"errors"
	"time"
	"unicode"
	"unicode/utf8"

	"cervterm/internal/ime"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
)

var (
	errCommittedTextInvalid = errors.New("committed text is invalid")
	errTextTargetStale      = errors.New("committed text target is stale")
	errTextTargetExhausted  = errors.New("committed text target counter exhausted")
)

const maxTextTargetSequence = ^uint64(0)

type committedTextTargetState struct {
	sequence       uint64
	paneActivation uint64
}

func (a *App) setFocusedPane(pane termmux.PaneID) {
	if a.focusedPane == pane {
		return
	}
	a.focusedPane = pane
	if a.textTarget.sequence == maxTextTargetSequence {
		a.textTarget.paneActivation = 0
		return
	}
	a.textTarget.sequence++
	a.textTarget.paneActivation = a.textTarget.sequence
}

func (a *App) captureCommittedTextTarget() (ime.Target, error) {
	if a.modal.Active() {
		state := a.modal.Snapshot()
		target := ime.Target{Kind: ime.TargetModal, ID: uint64(state.OpeningPane), Activation: uint64(state.Activation)}
		if !target.Valid() {
			return ime.Target{}, errTextTargetStale
		}
		return target, nil
	}
	if a.search.active {
		target := ime.Target{Kind: ime.TargetSearch, ID: uint64(a.focusedPane), Activation: uint64(a.search.activation)}
		if !target.Valid() {
			return ime.Target{}, errTextTargetStale
		}
		return target, nil
	}
	if a.focusedPane == 0 || a.mux == nil || a.textTarget.sequence == maxTextTargetSequence {
		if a.textTarget.sequence == maxTextTargetSequence {
			return ime.Target{}, errTextTargetExhausted
		}
		return ime.Target{}, errTextTargetStale
	}
	a.textTarget.sequence++
	a.textTarget.paneActivation = a.textTarget.sequence
	return ime.Target{Kind: ime.TargetPane, ID: uint64(a.focusedPane), Activation: a.textTarget.paneActivation}, nil
}

func (a *App) routeCommittedText(target ime.Target, text string) error {
	runes, err := validateCommittedText(text)
	if err != nil {
		return err
	}
	switch target.Kind {
	case ime.TargetModal:
		state := a.modal.Snapshot()
		if !a.modal.Active() || uint64(state.OpeningPane) != target.ID || uint64(state.Activation) != target.Activation {
			return errTextTargetStale
		}
		if len(state.Query)+len(runes) > modal.MaxQueryRunes {
			return errCommittedTextInvalid
		}
		if state.Mode == modal.ModeQuickSelect && len(runes) == 1 {
			a.handleQuickSelectChar(runes[0])
			return nil
		}
		before := a.modal.Revision()
		if !a.modal.AppendText(modal.ActivationID(target.Activation), text) {
			return errTextTargetStale
		}
		a.redrawModalMutation(before)
		return nil
	case ime.TargetSearch:
		if a.modal.Active() || !a.search.active || uint64(a.focusedPane) != target.ID || uint64(a.search.activation) != target.Activation {
			return errTextTargetStale
		}
		if len(a.search.query)+len(runes) > maxSearchQueryRunes {
			return errCommittedTextInvalid
		}
		if !a.search.appendText(searchActivationID(target.Activation), text) {
			return errTextTargetStale
		}
		return nil
	case ime.TargetPane:
		pane := termmux.PaneID(target.ID)
		if a.modal.Active() || a.search.active || a.mux == nil || pane == 0 || pane != a.focusedPane || target.Activation == 0 || target.Activation != a.textTarget.paneActivation {
			return errTextTargetStale
		}
		return a.writePaneInputBytesResult(pane, []byte(text))
	default:
		return errTextTargetStale
	}
}

func (a *App) routeGLFWChar(char rune) {
	if a.charSuppression.consume(char, time.Now()) {
		return
	}
	target, err := a.captureCommittedTextTarget()
	if err == nil {
		err = a.routeCommittedText(target, string(char))
	}
	if err != nil {
		a.Notify("input: " + err.Error())
	}
}

func validateCommittedText(text string) ([]rune, error) {
	if text == "" || !utf8.ValidString(text) || len(text) > ime.MaxCommitBytes {
		return nil, errCommittedTextInvalid
	}
	runes := []rune(text)
	if len(runes) > ime.MaxCommitRunes {
		return nil, errCommittedTextInvalid
	}
	for _, r := range runes {
		if unicode.IsControl(r) {
			return nil, errCommittedTextInvalid
		}
	}
	return runes, nil
}
