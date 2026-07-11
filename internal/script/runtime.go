package script

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cervterm/internal/config"

	lua "github.com/yuin/gopher-lua"
)

type Binding struct {
	Spec Spec
	fn   *lua.LFunction
}

// Runtime owns a persistent Lua state and must only be used from the thread that
// created it. The GLFW frontend calls Load and Dispatch on its main loop thread.
type Runtime struct {
	state           *lua.LState
	bindings        []Binding
	dispatchTimeout time.Duration
}

func Load(path string, base config.Config) (config.Config, *Runtime, error) {
	luaPath := path
	if strings.HasSuffix(strings.ToLower(path), ".tl") {
		generated, err := config.GenerateTeal(path)
		if err != nil {
			return base, nil, err
		}
		luaPath = generated
	}
	state := lua.NewState(lua.Options{SkipOpenLibs: false})
	state.PreloadModule("cervterm", func(state *lua.LState) int {
		state.Push(state.NewTable())
		return 1
	})
	if err := state.DoFile(luaPath); err != nil {
		state.Close()
		return base, nil, err
	}
	value := state.Get(-1)
	root, ok := value.(*lua.LTable)
	if !ok {
		state.Close()
		return base, nil, fmt.Errorf("config must return a table, got %s", value.Type().String())
	}
	cfg := config.FromTable(base, root)
	bindings, err := loadBindings(root)
	if err != nil {
		state.Close()
		return base, nil, err
	}
	return cfg, &Runtime{state: state, bindings: bindings, dispatchTimeout: time.Second}, nil
}

func (r *Runtime) Bindings() []Spec {
	out := make([]Spec, len(r.bindings))
	for i, binding := range r.bindings {
		out[i] = binding.Spec
	}
	return out
}

func (r *Runtime) Dispatch(index int, host Host) error {
	if index < 0 || index >= len(r.bindings) {
		return fmt.Errorf("binding index %d out of range", index)
	}
	binding := r.bindings[index]
	ctx, cancel := context.WithTimeout(context.Background(), r.dispatchTimeout)
	defer cancel()
	r.state.SetContext(ctx)
	defer r.state.SetContext(context.Background())

	if err := r.state.CallByParam(lua.P{
		Fn:      binding.fn,
		NRet:    0,
		Protect: true,
	}, termTable(r.state, host)); err != nil {
		return fmt.Errorf("keys %s: %w", binding.Spec.String(), err)
	}
	return nil
}

func (r *Runtime) Close() {
	if r == nil || r.state == nil {
		return
	}
	r.state.Close()
	r.state = nil
}

func loadBindings(root *lua.LTable) ([]Binding, error) {
	value := root.RawGetString("keys")
	if value == lua.LNil {
		return nil, nil
	}
	keys, ok := value.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("keys must be a table")
	}
	bindings := make([]Binding, 0, keys.Len())
	for i := 1; i <= keys.Len(); i++ {
		entry, ok := keys.RawGetInt(i).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("keys[%d]: entry must be a table", i)
		}
		keyValue, ok := entry.RawGetString("key").(lua.LString)
		if !ok {
			return nil, fmt.Errorf("keys[%d]: key must be a string", i)
		}
		modsValue := ""
		if value := entry.RawGetString("mods"); value != lua.LNil {
			mods, ok := value.(lua.LString)
			if !ok {
				return nil, fmt.Errorf("keys[%d]: mods must be a string", i)
			}
			modsValue = string(mods)
		}
		action, ok := entry.RawGetString("action").(*lua.LFunction)
		if !ok {
			return nil, fmt.Errorf("keys[%d]: action must be a function", i)
		}
		spec, err := ParseSpec(string(keyValue), modsValue)
		if err != nil {
			return nil, fmt.Errorf("keys[%d]: %w", i, err)
		}
		bindings = append(bindings, Binding{Spec: spec, fn: action})
	}
	return bindings, nil
}
