package config

import lua "github.com/yuin/gopher-lua"

var declarativeIncludeGuardKey = &lua.LUserData{Value: "cervterm.config.declarative_include_guard"}

// DeclarativeIncludeActive reports whether the current Lua call is evaluating a
// declarative include. Imperative cervterm module registrations must reject while
// this guard is active, including through nested require/dofile/loadfile calls.
func DeclarativeIncludeActive(state *lua.LState) bool {
	value := state.G.Registry.RawGet(declarativeIncludeGuardKey)
	active, ok := value.(lua.LBool)
	return ok && bool(active)
}

func setDeclarativeIncludeGuard(state *lua.LState, active bool) func() {
	previous := state.G.Registry.RawGet(declarativeIncludeGuardKey)
	state.G.Registry.RawSet(declarativeIncludeGuardKey, lua.LBool(active))
	return func() {
		state.G.Registry.RawSet(declarativeIncludeGuardKey, previous)
	}
}
