package script

import (
	"cervterm/internal/config"

	lua "github.com/yuin/gopher-lua"
)

func rejectDeclarativeIncludeRegistration(state *lua.LState, operation string) {
	if config.DeclarativeIncludeActive(state) {
		state.RaiseError("cervterm.%s cannot register imperative state while evaluating a declarative include", operation)
	}
}
