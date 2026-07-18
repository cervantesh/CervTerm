package action

import (
	"errors"
	"fmt"
	"math"
)

// ID is the stable serialized identity of an action.
type ID string

const (
	IDCopySelection          ID = "copy_selection"
	IDPasteClipboard         ID = "paste_clipboard"
	IDToggleSearch           ID = "toggle_search"
	IDToggleStats            ID = "toggle_stats"
	IDActivateCommandPalette ID = "activate_command_palette"
	IDActivateQuickSelect    ID = "activate_quick_select"
	IDActivateLaunchMenu     ID = "activate_launch_menu"
	IDScroll                 ID = "scroll"
	IDZoom                   ID = "zoom"
	IDReloadConfig           ID = "reload_config"
	IDSplitPane              ID = "split_pane"
	IDFocusPane              ID = "focus_pane"
	IDClosePane              ID = "close_pane"
	IDMultiple               ID = "multiple"
	IDCallback               ID = "callback"
	IDResizePane             ID = "resize_pane"
	IDSwapPane               ID = "swap_pane"
	IDMovePane               ID = "move_pane"
)

var (
	ErrNotSerializable   = errors.New("action is not serializable")
	ErrTargetUnavailable = errors.New("action target is unavailable")
)

// Action is a closed set of immutable command values. Callers must pass values,
// not pointers; Envelope validation rejects pointer forms before dispatch.
type Action interface {
	ID() ID
	Validate() error
	action()
}

type CopySelection struct{}
type PasteClipboard struct{}
type ToggleSearch struct{}
type ToggleStats struct{}
type ReloadConfig struct{}
type ClosePane struct{}
type ActivateCommandPalette struct{}
type ActivateQuickSelect struct{}
type ActivateLaunchMenu struct{}

func (CopySelection) ID() ID          { return IDCopySelection }
func (PasteClipboard) ID() ID         { return IDPasteClipboard }
func (ToggleSearch) ID() ID           { return IDToggleSearch }
func (ToggleStats) ID() ID            { return IDToggleStats }
func (ReloadConfig) ID() ID           { return IDReloadConfig }
func (ClosePane) ID() ID              { return IDClosePane }
func (ActivateCommandPalette) ID() ID { return IDActivateCommandPalette }
func (ActivateQuickSelect) ID() ID    { return IDActivateQuickSelect }
func (ActivateLaunchMenu) ID() ID     { return IDActivateLaunchMenu }

func (CopySelection) Validate() error          { return nil }
func (PasteClipboard) Validate() error         { return nil }
func (ToggleSearch) Validate() error           { return nil }
func (ToggleStats) Validate() error            { return nil }
func (ReloadConfig) Validate() error           { return nil }
func (ClosePane) Validate() error              { return nil }
func (ActivateCommandPalette) Validate() error { return nil }
func (ActivateQuickSelect) Validate() error    { return nil }
func (ActivateLaunchMenu) Validate() error     { return nil }

func (CopySelection) action()          {}
func (PasteClipboard) action()         {}
func (ToggleSearch) action()           {}
func (ToggleStats) action()            {}
func (ReloadConfig) action()           {}
func (ClosePane) action()              {}
func (ActivateCommandPalette) action() {}
func (ActivateQuickSelect) action()    {}
func (ActivateLaunchMenu) action()     {}

type ScrollUnit string

const (
	ScrollLine   ScrollUnit = "line"
	ScrollPage   ScrollUnit = "page"
	ScrollBuffer ScrollUnit = "buffer"
)

// Scroll moves toward older content for positive amounts and toward the bottom
// for negative amounts. Buffer amounts are restricted to -1 or 1.
type Scroll struct {
	Unit   ScrollUnit
	Amount int
}

func (Scroll) ID() ID  { return IDScroll }
func (Scroll) action() {}
func (a Scroll) Validate() error {
	if a.Unit != ScrollLine && a.Unit != ScrollPage && a.Unit != ScrollBuffer {
		return fmt.Errorf("scroll unit %q is invalid", a.Unit)
	}
	if a.Amount == 0 {
		return errors.New("scroll amount must not be zero")
	}
	if a.Unit == ScrollBuffer && a.Amount != -1 && a.Amount != 1 {
		return errors.New("buffer scroll amount must be -1 or 1")
	}
	return nil
}

type ZoomMode string

const (
	ZoomDelta ZoomMode = "delta"
	ZoomReset ZoomMode = "reset"
)

type Zoom struct {
	Mode   ZoomMode
	Amount float64
}

func (Zoom) ID() ID  { return IDZoom }
func (Zoom) action() {}
func (a Zoom) Validate() error {
	if math.IsNaN(a.Amount) || math.IsInf(a.Amount, 0) {
		return errors.New("zoom amount must be finite")
	}
	switch a.Mode {
	case ZoomDelta:
		if a.Amount == 0 {
			return errors.New("zoom delta must not be zero")
		}
	case ZoomReset:
		if a.Amount != 0 {
			return errors.New("zoom reset amount must be zero")
		}
	default:
		return fmt.Errorf("zoom mode %q is invalid", a.Mode)
	}
	return nil
}

type SplitAxis string

const (
	SplitColumns SplitAxis = "columns"
	SplitRows    SplitAxis = "rows"
)

type SplitPane struct{ Axis SplitAxis }

func (SplitPane) ID() ID  { return IDSplitPane }
func (SplitPane) action() {}
func (a SplitPane) Validate() error {
	if a.Axis != SplitColumns && a.Axis != SplitRows {
		return fmt.Errorf("split axis %q is invalid", a.Axis)
	}
	return nil
}

type Direction string

const (
	FocusLeft  Direction = "left"
	FocusRight Direction = "right"
	FocusUp    Direction = "up"
	FocusDown  Direction = "down"
)

type FocusPane struct{ Direction Direction }

func (FocusPane) ID() ID  { return IDFocusPane }
func (FocusPane) action() {}
func (a FocusPane) Validate() error {
	if err := validateDirection(a.Direction); err != nil {
		return fmt.Errorf("focus %w", err)
	}
	return nil
}

const (
	MaxSequenceActions = 64
	MaxSequenceDepth   = 32
	MaxActionNodes     = 4096
	MaxResizePaneDelta = 1024
)

type ResizePane struct {
	Direction Direction
	Delta     int
}

func (ResizePane) ID() ID  { return IDResizePane }
func (ResizePane) action() {}
func (a ResizePane) Validate() error {
	if err := validateDirection(a.Direction); err != nil {
		return fmt.Errorf("resize pane: %w", err)
	}
	if a.Delta <= 0 || a.Delta > MaxResizePaneDelta {
		return fmt.Errorf("resize pane delta must be in [1, %d] cells", MaxResizePaneDelta)
	}
	return nil
}

type SwapPane struct{ Direction Direction }

func (SwapPane) ID() ID  { return IDSwapPane }
func (SwapPane) action() {}
func (a SwapPane) Validate() error {
	if err := validateDirection(a.Direction); err != nil {
		return fmt.Errorf("swap pane: %w", err)
	}
	return nil
}

type MovePane struct{ Direction Direction }

func (MovePane) ID() ID  { return IDMovePane }
func (MovePane) action() {}
func (a MovePane) Validate() error {
	if err := validateDirection(a.Direction); err != nil {
		return fmt.Errorf("move pane: %w", err)
	}
	return nil
}

func validateDirection(direction Direction) error {
	switch direction {
	case FocusLeft, FocusRight, FocusUp, FocusDown:
		return nil
	default:
		return fmt.Errorf("direction %q is invalid", direction)
	}
}

// Multiple owns a copy of its children and returns copies to callers.
type Multiple struct{ actions []Envelope }

func NewMultiple(actions ...Envelope) (Multiple, error) {
	multiple := Multiple{actions: append([]Envelope(nil), actions...)}
	if err := multiple.Validate(); err != nil {
		return Multiple{}, err
	}
	return multiple, nil
}

func (a Multiple) Actions() []Envelope { return append([]Envelope(nil), a.actions...) }
func (Multiple) ID() ID                { return IDMultiple }
func (Multiple) action()               {}
func (a Multiple) Validate() error {
	nodes := 1
	return validateMultiple(a, 0, &nodes)
}

func validateMultiple(a Multiple, depth int, nodes *int) error {
	if depth > MaxSequenceDepth {
		return fmt.Errorf("multiple nesting exceeds maximum depth %d", MaxSequenceDepth)
	}
	if len(a.actions) == 0 {
		return errors.New("multiple requires at least one action")
	}
	if len(a.actions) > MaxSequenceActions {
		return fmt.Errorf("multiple has %d actions; maximum is %d", len(a.actions), MaxSequenceActions)
	}
	for i, child := range a.actions {
		if err := child.validateDepth(depth+1, nodes); err != nil {
			return fmt.Errorf("multiple action %d: %w", i, err)
		}
	}
	return nil
}

// Callback identifies one function in the current script runtime. It is valid
// for dispatch but deliberately excluded from serialization and reload reuse.
type Callback struct {
	BindingIndex int
	Label        string
}

func (Callback) ID() ID  { return IDCallback }
func (Callback) action() {}
func (a Callback) Validate() error {
	if a.BindingIndex < 0 {
		return errors.New("callback binding index must not be negative")
	}
	return nil
}

// actionIdentity accepts only the exact value types in the closed action set.
// This rejects pointers (including typed nil pointers) before method calls.
func actionIdentity(action Action) (ID, error) {
	switch action.(type) {
	case CopySelection:
		return IDCopySelection, nil
	case PasteClipboard:
		return IDPasteClipboard, nil
	case ToggleSearch:
		return IDToggleSearch, nil
	case ToggleStats:
		return IDToggleStats, nil
	case ActivateCommandPalette:
		return IDActivateCommandPalette, nil
	case ActivateQuickSelect:
		return IDActivateQuickSelect, nil
	case ActivateLaunchMenu:
		return IDActivateLaunchMenu, nil
	case Scroll:
		return IDScroll, nil
	case Zoom:
		return IDZoom, nil
	case ReloadConfig:
		return IDReloadConfig, nil
	case SplitPane:
		return IDSplitPane, nil
	case FocusPane:
		return IDFocusPane, nil
	case ResizePane:
		return IDResizePane, nil
	case SwapPane:
		return IDSwapPane, nil
	case MovePane:
		return IDMovePane, nil
	case ClosePane:
		return IDClosePane, nil
	case Multiple:
		return IDMultiple, nil
	case Callback:
		return IDCallback, nil
	default:
		return "", fmt.Errorf("action must be a concrete value, got %T", action)
	}
}
