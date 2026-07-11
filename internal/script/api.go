package script

import lua "github.com/yuin/gopher-lua"

type Host interface {
	WriteInput(data string)
	Notify(message string)
}

func termTable(state *lua.LState, host Host) *lua.LTable {
	tbl := state.NewTable()
	tbl.RawSetString("write", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.WriteInput(state.CheckString(2))
		return 0
	}))
	tbl.RawSetString("notify", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.Notify(state.CheckString(2))
		return 0
	}))
	return tbl
}
