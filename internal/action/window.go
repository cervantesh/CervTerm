package action

import (
	"errors"
	"fmt"
)

// Window actions always address stable process-local WindowIDs. Zero is invalid.
type NewWindow struct{}
type CloseWindow struct{ WindowID uint64 }
type FocusWindow struct{ WindowID uint64 }
type MoveTabToWindow struct {
	WindowID uint64
	TabID    uint64
	Position int
}
type MovePaneToWindow struct {
	WindowID uint64
	PaneID   uint64
	Axis     SplitAxis
}

func (NewWindow) ID() ID          { return IDNewWindow }
func (CloseWindow) ID() ID        { return IDCloseWindow }
func (FocusWindow) ID() ID        { return IDFocusWindow }
func (MoveTabToWindow) ID() ID    { return IDMoveTabToWindow }
func (MovePaneToWindow) ID() ID   { return IDMovePaneToWindow }
func (NewWindow) action()         {}
func (CloseWindow) action()       {}
func (FocusWindow) action()       {}
func (MoveTabToWindow) action()   {}
func (MovePaneToWindow) action()  {}
func (NewWindow) Validate() error { return nil }

func validateWindowID(id uint64) error {
	if id == 0 {
		return errors.New("window ID must be positive")
	}
	return nil
}
func (a CloseWindow) Validate() error { return validateWindowID(a.WindowID) }
func (a FocusWindow) Validate() error { return validateWindowID(a.WindowID) }
func (a MoveTabToWindow) Validate() error {
	if err := validateWindowID(a.WindowID); err != nil {
		return err
	}
	if err := validateTabID(a.TabID); err != nil {
		return err
	}
	if a.Position < 0 {
		return errors.New("move tab to window position must not be negative")
	}
	return nil
}
func (a MovePaneToWindow) Validate() error {
	if err := validateWindowID(a.WindowID); err != nil {
		return err
	}
	if a.PaneID == 0 {
		return errors.New("pane ID must be positive")
	}
	if a.Axis != SplitColumns && a.Axis != SplitRows {
		return fmt.Errorf("split axis %q is invalid", a.Axis)
	}
	return nil
}

type windowIDArgs struct {
	WindowID uint64 `json:"window_id"`
}
type moveTabToWindowArgs struct {
	WindowID uint64 `json:"window_id"`
	TabID    uint64 `json:"tab_id"`
	Position int    `json:"position"`
}
type movePaneToWindowArgs struct {
	WindowID uint64    `json:"window_id"`
	PaneID   uint64    `json:"pane_id"`
	Axis     SplitAxis `json:"axis"`
}

var closeWindowCodec = typedCodec("CloseWindow", func(a CloseWindow) windowIDArgs { return windowIDArgs{a.WindowID} }, func(a windowIDArgs) CloseWindow { return CloseWindow{a.WindowID} })
var focusWindowCodec = typedCodec("FocusWindow", func(a FocusWindow) windowIDArgs { return windowIDArgs{a.WindowID} }, func(a windowIDArgs) FocusWindow { return FocusWindow{a.WindowID} })
var moveTabToWindowCodec = typedCodec("MoveTabToWindow", func(a MoveTabToWindow) moveTabToWindowArgs {
	return moveTabToWindowArgs{a.WindowID, a.TabID, a.Position}
}, func(a moveTabToWindowArgs) MoveTabToWindow { return MoveTabToWindow{a.WindowID, a.TabID, a.Position} })
var movePaneToWindowCodec = typedCodec("MovePaneToWindow", func(a MovePaneToWindow) movePaneToWindowArgs {
	return movePaneToWindowArgs{a.WindowID, a.PaneID, a.Axis}
}, func(a movePaneToWindowArgs) MovePaneToWindow { return MovePaneToWindow{a.WindowID, a.PaneID, a.Axis} })
