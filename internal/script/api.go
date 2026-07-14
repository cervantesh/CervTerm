package script

import (
	"math"

	lua "github.com/yuin/gopher-lua"
)

// Host is the terminal surface exposed to Lua handlers. Read methods use 0-based
// row/column indices; the Lua boundary converts to 1-based.
type Host interface {
	WriteInput(data string)
	Notify(message string)
	Selection() string
	SetClipboard(text string)
	Clipboard() string
	Scroll(lines int) bool
	ScrollToBottom()
	ScrollbackLen() int
	Size() (cols, rows int)
	Cursor() (row, col int)
	Title() string
	SetTitle(title string)
	Line(row int) (string, bool)
	LineWrapped(row int) (bool, bool)
	FontSize() float64
	SetFontSize(pts float64)
}

// fontSizeMin and fontSizeMax bound term:set_font_size. Clamping at the Lua
// boundary keeps the frontend from having to re-validate and matches the plan.
const (
	fontSizeMin = 6.0
	fontSizeMax = 72.0
)

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
	tbl.RawSetString("selection", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LString(host.Selection()))
		return 1
	}))
	tbl.RawSetString("copy", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.SetClipboard(state.CheckString(2))
		return 0
	}))
	tbl.RawSetString("clipboard", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LString(host.Clipboard()))
		return 1
	}))
	tbl.RawSetString("scroll", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LBool(host.Scroll(state.CheckInt(2))))
		return 1
	}))
	tbl.RawSetString("scroll_to_bottom", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.ScrollToBottom()
		return 0
	}))
	tbl.RawSetString("scrollback", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LNumber(host.ScrollbackLen()))
		return 1
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
	tbl.RawSetString("set_title", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.SetTitle(state.CheckString(2))
		return 0
	}))
	tbl.RawSetString("line", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		row := state.CheckInt(2)
		// Out-of-range rows yield "" so the Lua return type is always a string.
		text, _ := host.Line(row - 1)
		state.Push(lua.LString(text))
		return 1
	}))
	tbl.RawSetString("line_wrapped", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		row := state.CheckInt(2)
		wrapped, _ := host.LineWrapped(row - 1)
		state.Push(lua.LBool(wrapped))
		return 1
	}))
	tbl.RawSetString("font_size", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LNumber(host.FontSize()))
		return 1
	}))
	tbl.RawSetString("set_font_size", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		pts := math.Max(fontSizeMin, math.Min(fontSizeMax, float64(state.CheckNumber(2))))
		host.SetFontSize(pts)
		return 0
	}))
	return tbl
}
