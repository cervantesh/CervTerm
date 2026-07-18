package script

import (
	"strings"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/config"
)

func TestLoadTabActions(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
return { keys = {
  { key = "n", action = cervterm.action.NewTab },
  { key = "a", action = cervterm.action.ActivateTab(1) },
  { key = "r", action = cervterm.action.ActivateTabRelative(-1) },
  { key = "m", action = cervterm.action.MoveTab(2, 0) },
  { key = "t", action = cervterm.action.RenameTab(3, "work") },
  { key = "c", action = cervterm.action.CloseTab(4) },
  { key = "p", action = cervterm.action.MovePaneToTab(5, "rows") },
  { key = "s", action = cervterm.action.ActivateTabSwitcher },
} }`)
	_, runtime, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	bindings := runtime.Bindings()
	if len(bindings) != 8 {
		t.Fatalf("bindings = %d", len(bindings))
	}
	if _, ok := bindings[0].Action.Action.(termaction.NewTab); !ok {
		t.Fatalf("new tab = %#v", bindings[0])
	}
	if got := bindings[1].Action.Action.(termaction.ActivateTab); got.TabID != 1 {
		t.Fatalf("activate = %#v", got)
	}
	if got := bindings[2].Action.Action.(termaction.ActivateTabRelative); got.Delta != -1 {
		t.Fatalf("relative = %#v", got)
	}
	if got := bindings[3].Action.Action.(termaction.MoveTab); got.TabID != 2 || got.Position != 0 {
		t.Fatalf("move = %#v", got)
	}
	if got := bindings[4].Action.Action.(termaction.RenameTab); got.TabID != 3 || got.Title != "work" {
		t.Fatalf("rename = %#v", got)
	}
	if got := bindings[5].Action.Action.(termaction.CloseTab); got.TabID != 4 {
		t.Fatalf("close = %#v", got)
	}
	if got := bindings[6].Action.Action.(termaction.MovePaneToTab); got.TabID != 5 || got.Axis != termaction.SplitRows {
		t.Fatalf("move pane = %#v", got)
	}
	if _, ok := bindings[7].Action.Action.(termaction.ActivateTabSwitcher); !ok {
		t.Fatalf("switcher = %#v", bindings[7])
	}
}

func TestTabActionConstructorsRejectUnsafeArguments(t *testing.T) {
	tests := []struct{ name, expression, want string }{
		{"zero ID", `cervterm.action.ActivateTab(0)`, "positive safe integer"},
		{"fractional ID", `cervterm.action.CloseTab(1.5)`, "positive safe integer"},
		{"unsafe ID", `cervterm.action.ActivateTab(9007199254740992)`, "positive safe integer"},
		{"zero relative", `cervterm.action.ActivateTabRelative(0)`, "must not be zero"},
		{"negative position", `cervterm.action.MoveTab(1, -1)`, "position"},
		{"bad axis", `cervterm.action.MovePaneToTab(1, "diagonal")`, "axis"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeScriptConfig(t, `local cervterm = require("cervterm")
return { keys = { { key = "a", action = `+tt.expression+` } } }`)
			_, runtime, err := Load(path, config.Defaults())
			if runtime != nil {
				runtime.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want %q", err, tt.want)
			}
		})
	}
}
