package script

import (
	"strings"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/config"
)

func TestLoadTypedActionsAndLegacyCallback(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
return {
  keys = {
    { key = "c", label = "Copy selected text", action = cervterm.action.CopySelection },
    { key = "p", action = cervterm.action.ScrollPage(1) },
    { key = "m", action = cervterm.action.Multiple({
      cervterm.action.FocusPane("left"),
      cervterm.action.ClosePane,
    }) },
    { key = "o", action = cervterm.action.WithTarget(cervterm.action.Zoom(2), "origin") },
    { key = "l", label = "Legacy callback", action = function(term) term:notify("legacy") end },
    { key = "r", action = cervterm.action.ResizePane("right", 3) },
    { key = "s", action = cervterm.action.SwapPane("left") },
    { key = "v", action = cervterm.action.MovePane("down") },
    { key = "n", action = cervterm.action.ScrollToPrompt(-1) },
    { key = "z", action = cervterm.action.CopySemanticZone("output") },
    { key = "y", action = cervterm.action.SelectSemanticZone("input") }
  },
}`)
	_, runtime, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	bindings := runtime.Bindings()
	if len(bindings) != 11 {
		t.Fatalf("bindings = %#v", bindings)
	}
	if _, ok := bindings[0].Action.Action.(termaction.CopySelection); !ok || bindings[0].Label != "Copy selected text" {
		t.Fatalf("copy binding = %#v", bindings[0])
	}
	if scroll, ok := bindings[1].Action.Action.(termaction.Scroll); !ok || scroll.Unit != termaction.ScrollPage || scroll.Amount != 1 {
		t.Fatalf("scroll binding = %#v", bindings[1])
	}
	multiple, ok := bindings[2].Action.Action.(termaction.Multiple)
	if !ok || len(multiple.Actions()) != 2 {
		t.Fatalf("multiple binding = %#v", bindings[2])
	}
	if bindings[3].Action.Target != termaction.TargetOrigin {
		t.Fatalf("origin binding target = %q", bindings[3].Action.Target)
	}
	callback, ok := bindings[4].Action.Action.(termaction.Callback)
	if !ok || callback.BindingIndex != 4 || callback.Label != "Legacy callback" {
		t.Fatalf("callback binding = %#v", bindings[4])
	}
	resize, ok := bindings[5].Action.Action.(termaction.ResizePane)
	if !ok || resize.Direction != termaction.FocusRight || resize.Delta != 3 {
		t.Fatalf("resize binding = %#v", bindings[5])
	}
	if swap, ok := bindings[6].Action.Action.(termaction.SwapPane); !ok || swap.Direction != termaction.FocusLeft {
		t.Fatalf("swap binding = %#v", bindings[6])
	}
	if move, ok := bindings[7].Action.Action.(termaction.MovePane); !ok || move.Direction != termaction.FocusDown {
		t.Fatalf("move binding = %#v", bindings[7])
	}
	if prompt, ok := bindings[8].Action.Action.(termaction.ScrollToPrompt); !ok || prompt.Delta != -1 {
		t.Fatalf("prompt binding = %#v", bindings[8])
	}
	if semantic, ok := bindings[9].Action.Action.(termaction.CopySemanticZone); !ok || semantic.Zone != termaction.SemanticZoneOutput {
		t.Fatalf("semantic binding = %#v", bindings[9])
	}
	if semantic, ok := bindings[10].Action.Action.(termaction.SelectSemanticZone); !ok || semantic.Zone != termaction.SemanticZoneInput {
		t.Fatalf("select semantic binding = %#v", bindings[10])
	}
	descriptor, err := termaction.DefaultRegistry().Describe(callback)
	if err != nil || !descriptor.Discoverable || descriptor.Label != "Legacy callback" {
		t.Fatalf("callback descriptor = %#v, %v", descriptor, err)
	}

	if err := runtime.Dispatch(0, &fakeHost{}); err == nil || !strings.Contains(err.Error(), "not a Lua callback") {
		t.Fatalf("typed Dispatch error = %v", err)
	}
	host := &fakeHost{}
	if err := runtime.Dispatch(4, host); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(host.notices, ""); got != "legacy" {
		t.Fatalf("callback notices = %q", got)
	}
}

func TestTypedActionConstructorsRejectInvalidArguments(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   string
	}{
		{name: "zero scroll", action: `cervterm.action.ScrollPage(0)`, want: "must not be zero"},
		{name: "fractional scroll", action: `cervterm.action.ScrollPage(1.9)`, want: "integer"},
		{name: "overflow scroll", action: `cervterm.action.ScrollPage(1e100)`, want: "integer"},
		{name: "bad prompt delta", action: `cervterm.action.ScrollToPrompt(2)`, want: "-1 or 1"},
		{name: "bad semantic zone", action: `cervterm.action.CopySemanticZone("prompt")`, want: "input or output"},
		{name: "bad select semantic zone", action: `cervterm.action.SelectSemanticZone("prompt")`, want: "input or output"},
		{name: "bad split", action: `cervterm.action.SplitPane("diagonal")`, want: "split axis"},
		{name: "bad focus", action: `cervterm.action.FocusPane("next")`, want: "focus direction"},
		{name: "bad resize direction", action: `cervterm.action.ResizePane("next", 1)`, want: "direction"},
		{name: "zero resize", action: `cervterm.action.ResizePane("left", 0)`, want: "delta"},
		{name: "oversized resize", action: `cervterm.action.ResizePane("left", 1025)`, want: "delta"},
		{name: "bad swap", action: `cervterm.action.SwapPane("next")`, want: "direction"},
		{name: "bad move", action: `cervterm.action.MovePane("next")`, want: "direction"},
		{name: "empty multiple", action: `cervterm.action.Multiple({})`, want: "at least one"},
		{name: "bad target", action: `cervterm.action.WithTarget(cervterm.action.CopySelection, "pane-9")`, want: "target"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeScriptConfig(t, `local cervterm = require("cervterm")
return { keys = { { key = "a", action = `+tt.action+` } } }`)
			_, runtime, err := Load(path, config.Defaults())
			if runtime != nil {
				runtime.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsNonActionUserdataAndBadLabel(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "foreign userdata", source: `return { keys = { { key = "a", action = io.stdout } } }`, want: "not a cervterm action"},
		{name: "bad label", source: `return { keys = { { key = "a", label = 3, action = function() end } } }`, want: "label must be a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeScriptConfig(t, tt.source)
			_, runtime, err := Load(path, config.Defaults())
			if runtime != nil {
				runtime.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestLuaActionValuesAreImmutable(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
local action = cervterm.action.CopySelection
if getmetatable(action) ~= "locked" then error("action metatable is exposed") end
local ok = pcall(function() action.extra = true end)
if ok then error("action userdata was mutable") end
return { keys = { { key = "c", action = action } } }`)
	_, runtime, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runtime.Close()
}

func TestCallbackIdentityIsRuntimeLocalAfterReload(t *testing.T) {
	firstPath := writeScriptConfig(t, `return { keys = { { key = "a", action = function(term) term:notify("first") end } } }`)
	_, first, err := Load(firstPath, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	secondPath := writeScriptConfig(t, `local cervterm = require("cervterm")
return { keys = {
	{ key = "c", action = cervterm.action.CopySelection },
	{ key = "b", action = function(term) term:notify("second") end },
} }`)
	_, second, err := Load(secondPath, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	firstCallback := first.Bindings()[0].Action.Action.(termaction.Callback)
	secondCallback := second.Bindings()[1].Action.Action.(termaction.Callback)
	if firstCallback.BindingIndex != 0 || secondCallback.BindingIndex != 1 {
		t.Fatalf("callback identities: first=%#v second=%#v", firstCallback, secondCallback)
	}
	firstHost, secondHost := &fakeHost{}, &fakeHost{}
	if err := first.Dispatch(firstCallback.BindingIndex, firstHost); err != nil {
		t.Fatal(err)
	}
	if err := second.Dispatch(secondCallback.BindingIndex, secondHost); err != nil {
		t.Fatal(err)
	}
	if strings.Join(firstHost.notices, "") != "first" || strings.Join(secondHost.notices, "") != "second" {
		t.Fatalf("runtime-local dispatch: first=%v second=%v", firstHost.notices, secondHost.notices)
	}
}
