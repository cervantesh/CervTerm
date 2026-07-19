package script

import (
	"fmt"
	"math"
	"strconv"

	termaction "cervterm/internal/action"

	lua "github.com/yuin/gopher-lua"
)

const luaActionTypeName = "cervterm.action"

func installActionModule(state *lua.LState, module *lua.LTable) {
	metatable := state.NewTypeMetatable(luaActionTypeName)
	state.SetField(metatable, "__metatable", lua.LString("locked"))
	state.SetField(metatable, "__tostring", state.NewFunction(func(l *lua.LState) int {
		envelope := checkLuaAction(l, 1)
		l.Push(lua.LString(fmt.Sprintf("cervterm.action.%s(%s)", envelope.Action.ID(), envelope.Target)))
		return 1
	}))

	actions := state.NewTable()
	setActionConstant(state, actions, "CopySelection", termaction.CopySelection{})
	setActionConstant(state, actions, "PasteClipboard", termaction.PasteClipboard{})
	setActionConstant(state, actions, "ToggleSearch", termaction.ToggleSearch{})
	setActionConstant(state, actions, "ActivateCommandPalette", termaction.ActivateCommandPalette{})
	setActionConstant(state, actions, "ActivateQuickSelect", termaction.ActivateQuickSelect{})
	setActionConstant(state, actions, "ActivateLaunchMenu", termaction.ActivateLaunchMenu{})
	setActionConstant(state, actions, "ToggleStats", termaction.ToggleStats{})
	setActionConstant(state, actions, "ReloadConfig", termaction.ReloadConfig{})
	setActionConstant(state, actions, "ClosePane", termaction.ClosePane{})
	setActionConstant(state, actions, "ResetFontSize", termaction.Zoom{Mode: termaction.ZoomReset})
	setActionConstant(state, actions, "NewTab", termaction.NewTab{})
	setActionConstant(state, actions, "ActivateTabSwitcher", termaction.ActivateTabSwitcher{})
	setActionConstant(state, actions, "NewWindow", termaction.NewWindow{})
	setActionConstant(state, actions, "ActivateWorkspaceSwitcher", termaction.ActivateWorkspaceSwitcher{})

	actions.RawSetString("ScrollLines", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.Scroll{Unit: termaction.ScrollLine, Amount: checkLuaActionInt(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("ScrollPage", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.Scroll{Unit: termaction.ScrollPage, Amount: checkLuaActionInt(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("ScrollBuffer", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.Scroll{Unit: termaction.ScrollBuffer, Amount: checkLuaActionInt(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("ScrollToPrompt", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.ScrollToPrompt{Delta: checkLuaActionInt(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("Zoom", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.Zoom{Mode: termaction.ZoomDelta, Amount: float64(l.CheckNumber(1))}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("SplitPane", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.SplitPane{Axis: termaction.SplitAxis(l.CheckString(1))}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("FocusPane", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.FocusPane{Direction: termaction.Direction(l.CheckString(1))}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("ResizePane", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.ResizePane{Direction: termaction.Direction(l.CheckString(1)), Delta: checkLuaActionInt(l, 2)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("SwapPane", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.SwapPane{Direction: termaction.Direction(l.CheckString(1))}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("MovePane", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.MovePane{Direction: termaction.Direction(l.CheckString(1))}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("ActivateTab", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.ActivateTab{TabID: checkLuaTabID(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("ActivateTabRelative", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.ActivateTabRelative{Delta: checkLuaActionInt(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("MoveTab", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.MoveTab{TabID: checkLuaTabID(l, 1), Position: checkLuaActionInt(l, 2)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("RenameTab", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.RenameTab{TabID: checkLuaTabID(l, 1), Title: l.CheckString(2)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("CloseTab", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.CloseTab{TabID: checkLuaTabID(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("MovePaneToTab", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.MovePaneToTab{TabID: checkLuaTabID(l, 1), Axis: termaction.SplitAxis(l.CheckString(2))}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("CloseWindow", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.CloseWindow{WindowID: checkLuaWindowID(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("FocusWindow", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.FocusWindow{WindowID: checkLuaWindowID(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("MoveTabToWindow", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.MoveTabToWindow{WindowID: checkLuaWindowID(l, 1), TabID: checkLuaTabID(l, 2), Position: checkLuaActionInt(l, 3)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("MovePaneToWindow", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.MovePaneToWindow{WindowID: checkLuaWindowID(l, 1), PaneID: checkLuaTabID(l, 2), Axis: termaction.SplitAxis(l.CheckString(3))}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("CreateWorkspace", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.CreateWorkspace{Name: l.CheckString(1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("SwitchWorkspace", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.SwitchWorkspace{WorkspaceID: checkLuaWorkspaceID(l, 1)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("RenameWorkspace", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.RenameWorkspace{WorkspaceID: checkLuaWorkspaceID(l, 1), Name: l.CheckString(2)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("MoveWindowToWorkspace", state.NewFunction(func(l *lua.LState) int {
		pushLuaAction(l, termaction.MoveWindowToWorkspace{WindowID: checkLuaWindowID(l, 1), WorkspaceID: checkLuaWorkspaceID(l, 2)}, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("Multiple", state.NewFunction(func(l *lua.LState) int {
		values := l.CheckTable(1)
		children := make([]termaction.Envelope, values.Len())
		for i := 1; i <= values.Len(); i++ {
			value := values.RawGetInt(i)
			child, ok := luaAction(value)
			if !ok {
				l.RaiseError("Multiple action %d must be a cervterm action, got %s", i, value.Type().String())
			}
			children[i-1] = child
		}
		multiple, err := termaction.NewMultiple(children...)
		if err != nil {
			l.RaiseError("invalid Multiple action: %v", err)
		}
		pushLuaAction(l, multiple, termaction.TargetFocused)
		return 1
	}))
	actions.RawSetString("WithTarget", state.NewFunction(func(l *lua.LState) int {
		envelope := checkLuaAction(l, 1)
		envelope.Target = termaction.TargetSelector(l.CheckString(2))
		if err := envelope.Validate(); err != nil {
			l.RaiseError("invalid action target: %v", err)
		}
		pushLuaEnvelope(l, envelope)
		return 1
	}))
	module.RawSetString("action", actions)
}

func setActionConstant(state *lua.LState, table *lua.LTable, name string, command termaction.Action) {
	envelope, err := termaction.New(command, termaction.TargetFocused)
	if err != nil {
		panic(err)
	}
	table.RawSetString(name, newLuaAction(state, envelope))
}

func pushLuaAction(state *lua.LState, command termaction.Action, target termaction.TargetSelector) {
	envelope, err := termaction.New(command, target)
	if err != nil {
		state.RaiseError("invalid action: %v", err)
	}
	pushLuaEnvelope(state, envelope)
}

func pushLuaEnvelope(state *lua.LState, envelope termaction.Envelope) {
	state.Push(newLuaAction(state, envelope))
}

func newLuaAction(state *lua.LState, envelope termaction.Envelope) *lua.LUserData {
	userdata := state.NewUserData()
	userdata.Value = envelope
	state.SetMetatable(userdata, state.GetTypeMetatable(luaActionTypeName))
	return userdata
}

func checkLuaAction(state *lua.LState, index int) termaction.Envelope {
	value := state.CheckAny(index)
	envelope, ok := luaAction(value)
	if !ok {
		state.ArgError(index, "cervterm action expected")
	}
	return envelope
}

func luaAction(value lua.LValue) (termaction.Envelope, bool) {
	userdata, ok := value.(*lua.LUserData)
	if !ok {
		return termaction.Envelope{}, false
	}
	envelope, ok := userdata.Value.(termaction.Envelope)
	return envelope, ok
}

func checkLuaActionInt(state *lua.LState, index int) int {
	value := float64(state.CheckNumber(index))
	upperBound := math.Ldexp(1, strconv.IntSize-1)
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < -upperBound || value >= upperBound {
		state.ArgError(index, fmt.Sprintf("integer in [%g, %g) expected", -upperBound, upperBound))
	}
	return int(value)
}

func checkLuaTabID(state *lua.LState, index int) uint64 {
	const maxSafeInteger = uint64(1<<53 - 1)
	value := float64(state.CheckNumber(index))
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < 1 || value > float64(maxSafeInteger) {
		state.ArgError(index, fmt.Sprintf("positive safe integer in [1, %d] expected", maxSafeInteger))
	}
	return uint64(value)
}

func checkLuaWindowID(state *lua.LState, index int) uint64 {
	return checkLuaTabID(state, index)
}

func checkLuaWorkspaceID(state *lua.LState, index int) uint64 {
	return checkLuaTabID(state, index)
}
