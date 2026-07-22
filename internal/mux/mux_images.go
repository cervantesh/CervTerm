package mux

import (
	"cervterm/internal/kitty"
	"cervterm/internal/sixel"
	"cervterm/internal/termimage"
	"cervterm/internal/vt"
)

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
	if m.options.KittyEnabled {
		pane.kittyAdapter = kitty.NewAdapter(store)
	}
	if m.options.SixelEnabled {
		pane.sixelAdapter = sixel.NewAdapter(store)
	}
	if pane.kittyAdapter != nil || pane.sixelAdapter != nil {
		pane.parser.SetControlStringSink(func(event vt.ControlStringEvent) {
			switch event.Kind {
			case vt.ControlStringAPC:
				if pane.kittyAdapter == nil {
					return
				}
				outcome := pane.kittyAdapter.Advance(m.options.Now(), kitty.APCEvent{Data: event.Chunk, Final: event.Final, Cancelled: event.Cancelled, Overflow: event.Overflow})
				if outcome.Command != nil || outcome.Failure != kitty.ReplyNone {
					pane.kittyOutcomes = append(pane.kittyOutcomes, outcome)
					pane.kittyEvents = append(pane.kittyEvents, m.processKittyOutcomes(pane)...)
				}
			case vt.ControlStringDCS:
				if pane.sixelAdapter == nil {
					return
				}
				outcome := pane.sixelAdapter.Advance(m.options.Now(), sixel.DCSEvent{Data: event.Chunk, Final: event.Final, Cancelled: event.Cancelled, Overflow: event.Overflow})
				if outcome.Command != nil || outcome.Failure != sixel.FailureNone {
					pane.sixelOutcomes = append(pane.sixelOutcomes, outcome)
					m.processSixelOutcomes(pane)
				}
			}
		})
	}
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
