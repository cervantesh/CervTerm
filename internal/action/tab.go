package action

import (
	"encoding/json"
	"errors"
	"fmt"
)

type NewTab struct{}
type ActivateTab struct{ TabID uint64 }
type ActivateTabRelative struct{ Delta int }
type MoveTab struct {
	TabID    uint64
	Position int
}
type RenameTab struct {
	TabID uint64
	Title string
}
type CloseTab struct{ TabID uint64 }
type MovePaneToTab struct {
	TabID uint64
	Axis  SplitAxis
}
type ActivateTabSwitcher struct{}

func (NewTab) ID() ID              { return IDNewTab }
func (ActivateTab) ID() ID         { return IDActivateTab }
func (ActivateTabRelative) ID() ID { return IDActivateTabRelative }
func (MoveTab) ID() ID             { return IDMoveTab }
func (RenameTab) ID() ID           { return IDRenameTab }
func (CloseTab) ID() ID            { return IDCloseTab }
func (MovePaneToTab) ID() ID       { return IDMovePaneToTab }
func (ActivateTabSwitcher) ID() ID { return IDActivateTabSwitcher }

func (NewTab) action()              {}
func (ActivateTab) action()         {}
func (ActivateTabRelative) action() {}
func (MoveTab) action()             {}
func (RenameTab) action()           {}
func (CloseTab) action()            {}
func (MovePaneToTab) action()       {}
func (ActivateTabSwitcher) action() {}

func (NewTab) Validate() error              { return nil }
func (ActivateTabSwitcher) Validate() error { return nil }
func (a ActivateTab) Validate() error       { return validateTabID(a.TabID) }
func (a CloseTab) Validate() error          { return validateTabID(a.TabID) }
func (a ActivateTabRelative) Validate() error {
	if a.Delta == 0 {
		return errors.New("activate tab relative delta must not be zero")
	}
	return nil
}
func (a MoveTab) Validate() error {
	if err := validateTabID(a.TabID); err != nil {
		return err
	}
	if a.Position < 0 {
		return errors.New("move tab position must not be negative")
	}
	return nil
}
func (a RenameTab) Validate() error { return validateTabID(a.TabID) }
func (a MovePaneToTab) Validate() error {
	if err := validateTabID(a.TabID); err != nil {
		return err
	}
	if a.Axis != SplitColumns && a.Axis != SplitRows {
		return fmt.Errorf("split axis %q is invalid", a.Axis)
	}
	return nil
}
func validateTabID(id uint64) error {
	if id == 0 {
		return errors.New("tab ID must be positive")
	}
	return nil
}

type tabIDArgs struct {
	TabID uint64 `json:"tab_id"`
}
type tabRelativeArgs struct {
	Delta int `json:"delta"`
}
type moveTabArgs struct {
	TabID    uint64 `json:"tab_id"`
	Position int    `json:"position"`
}
type renameTabArgs struct {
	TabID uint64 `json:"tab_id"`
	Title string `json:"title"`
}
type movePaneToTabArgs struct {
	TabID uint64    `json:"tab_id"`
	Axis  SplitAxis `json:"axis"`
}

func typedCodec[T Action, A any](name string, args func(T) A, makeAction func(A) T) codecOps {
	return codecOps{
		encode: func(action Action, _ *Codec, _ int, _ *codecBudget) (json.RawMessage, error) {
			value, ok := action.(T)
			if !ok {
				return nil, fmt.Errorf("expected %s, got %T", name, action)
			}
			return json.Marshal(args(value))
		},
		decode: func(data json.RawMessage, _ *Codec, _ int, _ *codecBudget) (Action, error) {
			var value A
			if err := decodeObject(data, &value); err != nil {
				return nil, err
			}
			return makeAction(value), nil
		},
	}
}

var activateTabCodec = typedCodec("ActivateTab", func(a ActivateTab) tabIDArgs { return tabIDArgs{a.TabID} }, func(a tabIDArgs) ActivateTab { return ActivateTab{a.TabID} })
var activateTabRelativeCodec = typedCodec("ActivateTabRelative", func(a ActivateTabRelative) tabRelativeArgs { return tabRelativeArgs{a.Delta} }, func(a tabRelativeArgs) ActivateTabRelative { return ActivateTabRelative{a.Delta} })
var moveTabCodec = typedCodec("MoveTab", func(a MoveTab) moveTabArgs { return moveTabArgs{a.TabID, a.Position} }, func(a moveTabArgs) MoveTab { return MoveTab{a.TabID, a.Position} })
var renameTabCodec = typedCodec("RenameTab", func(a RenameTab) renameTabArgs { return renameTabArgs{a.TabID, a.Title} }, func(a renameTabArgs) RenameTab { return RenameTab{a.TabID, a.Title} })
var closeTabCodec = typedCodec("CloseTab", func(a CloseTab) tabIDArgs { return tabIDArgs{a.TabID} }, func(a tabIDArgs) CloseTab { return CloseTab{a.TabID} })
var movePaneToTabCodec = typedCodec("MovePaneToTab", func(a MovePaneToTab) movePaneToTabArgs { return movePaneToTabArgs{a.TabID, a.Axis} }, func(a movePaneToTabArgs) MovePaneToTab { return MovePaneToTab{a.TabID, a.Axis} })
