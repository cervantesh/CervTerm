package script

import lua "github.com/yuin/gopher-lua"

// Host is the terminal surface exposed to Lua handlers. Read methods use 0-based
// row/column indices; the Lua boundary converts to 1-based.
type Host interface {
	WriteInput(data string)
	Notify(message string)
	Size() (cols, rows int)
	Cursor() (row, col int)
	Title() string
	Line(row int) (string, bool)
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
	tbl.RawSetString("size", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cols, rows := host.Size()
		state.Push(lua.LNumber(cols))
		state.Push(lua.LNumber(rows))
		return 2
	}))
	tbl.RawSetString("cursor", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		row, col := host.Cursor()
		state.Push(lua.LNumber(row + 1))
		state.Push(lua.LNumber(col + 1))
		return 2
	}))
	tbl.RawSetString("title", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LString(host.Title()))
		return 1
	}))
	tbl.RawSetString("line", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		row := state.CheckInt(2)
		text, ok := host.Line(row - 1)
		if !ok {
			state.Push(lua.LNil)
			return 1
		}
		state.Push(lua.LString(text))
		return 1
	}))
	return tbl
}
