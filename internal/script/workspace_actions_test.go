package script

import (
	"os"
	"path/filepath"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/config"
)

func TestLuaWorkspaceActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	source := `local c=require("cervterm")
return {keys={
 {key="o",action=c.action.ActivateWorkspaceSwitcher},
 {key="c",action=c.action.CreateWorkspace("build")},
 {key="s",action=c.action.SwitchWorkspace(2)},
 {key="r",action=c.action.RenameWorkspace(2,"ops")},
 {key="m",action=c.action.MoveWindowToWorkspace(3,2)},
}}`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	_, runtime, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	bindings := runtime.BindingSet().Root
	if len(bindings) != 5 {
		t.Fatalf("bindings=%d", len(bindings))
	}
	if _, ok := bindings[0].Action.Action.(termaction.ActivateWorkspaceSwitcher); !ok {
		t.Fatalf("switcher=%T", bindings[0].Action.Action)
	}
	if got := bindings[1].Action.Action.(termaction.CreateWorkspace); got.Name != "build" {
		t.Fatalf("create=%#v", got)
	}
	if got := bindings[2].Action.Action.(termaction.SwitchWorkspace); got.WorkspaceID != 2 {
		t.Fatalf("switch=%#v", got)
	}
	if got := bindings[3].Action.Action.(termaction.RenameWorkspace); got.WorkspaceID != 2 || got.Name != "ops" {
		t.Fatalf("rename=%#v", got)
	}
	if got := bindings[4].Action.Action.(termaction.MoveWindowToWorkspace); got.WindowID != 3 || got.WorkspaceID != 2 {
		t.Fatalf("move=%#v", got)
	}
}

func TestLuaWorkspaceActionsRejectInvalidArguments(t *testing.T) {
	for _, expression := range []string{
		`c.action.CreateWorkspace("")`,
		`c.action.SwitchWorkspace(0)`,
		`c.action.RenameWorkspace(1,"")`,
		`c.action.MoveWindowToWorkspace(0,1)`,
		`c.action.MoveWindowToWorkspace(1,9007199254740992)`,
	} {
		path := filepath.Join(t.TempDir(), "bad.lua")
		source := `local c=require("cervterm"); return {keys={{key="x",action=` + expression + `}}}`
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if _, runtime, err := Load(path, config.Defaults()); err == nil {
			runtime.Close()
			t.Fatalf("accepted %s", expression)
		}
	}
}
