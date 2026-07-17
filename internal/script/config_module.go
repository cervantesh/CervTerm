package script

import (
	"cervterm/internal/config"

	lua "github.com/yuin/gopher-lua"
)

func installConfigModule(state *lua.LState, module *lua.LTable) {
	configModule := state.NewTable()
	configModule.RawSetString("unset", config.NewUnsetValue(state))
	module.RawSetString("config", configModule)
}
