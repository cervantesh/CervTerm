package script

import (
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/config"
)

func TestCommandPaletteActionLoadsFromLua(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm=require("cervterm")
return { keys={{key="p",mods="ctrl+shift",label="Commands",action=cervterm.action.ActivateCommandPalette}} }`)
	_, runtime, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	bindings := runtime.Bindings()
	if len(bindings) != 1 {
		t.Fatalf("bindings=%#v", bindings)
	}
	if _, ok := bindings[0].Action.Action.(termaction.ActivateCommandPalette); !ok {
		t.Fatalf("action=%T", bindings[0].Action.Action)
	}
}
