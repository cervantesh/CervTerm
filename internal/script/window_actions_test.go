package script

import (
	"os"
	"path/filepath"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/config"
)

func TestLuaWindowActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	source := `local c=require("cervterm")
return { keys={
 {key="n",action=c.action.NewWindow},
 {key="c",action=c.action.CloseWindow(2)},
 {key="f",action=c.action.FocusWindow(3)},
 {key="t",action=c.action.MoveTabToWindow(4,5,1)},
 {key="p",action=c.action.MovePaneToWindow(6,7,"rows")},
} }`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	bindings := rt.BindingSet().Root
	if len(bindings) != 5 {
		t.Fatalf("bindings=%d", len(bindings))
	}
	if _, ok := bindings[0].Action.Action.(termaction.NewWindow); !ok {
		t.Fatalf("new=%T", bindings[0].Action.Action)
	}
	if got := bindings[1].Action.Action.(termaction.CloseWindow); got.WindowID != 2 {
		t.Fatalf("close=%#v", got)
	}
	if got := bindings[2].Action.Action.(termaction.FocusWindow); got.WindowID != 3 {
		t.Fatalf("focus=%#v", got)
	}
	if got := bindings[3].Action.Action.(termaction.MoveTabToWindow); got.WindowID != 4 || got.TabID != 5 || got.Position != 1 {
		t.Fatalf("tab=%#v", got)
	}
	if got := bindings[4].Action.Action.(termaction.MovePaneToWindow); got.WindowID != 6 || got.PaneID != 7 || got.Axis != termaction.SplitRows {
		t.Fatalf("pane=%#v", got)
	}
}

func TestLuaWindowActionsRejectUnsafeIDs(t *testing.T) {
	for _, expr := range []string{`c.action.CloseWindow(0)`, `c.action.FocusWindow(9007199254740992)`, `c.action.MoveTabToWindow(1,2,-1)`, `c.action.MovePaneToWindow(1,2,"diagonal")`} {
		path := filepath.Join(t.TempDir(), "bad.lua")
		src := `local c=require("cervterm"); return {keys={{key="x",action=` + expr + `}}}`
		_ = os.WriteFile(path, []byte(src), 0600)
		if _, rt, err := Load(path, config.Defaults()); err == nil {
			rt.Close()
			t.Fatalf("accepted %s", expr)
		}
	}
}
