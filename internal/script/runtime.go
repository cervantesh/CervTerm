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

// eventHandlers holds the optional Lua callbacks fired by terminal events.
type eventHandlers struct {
	output *lua.LFunction
	title  *lua.LFunction
	cwd    *lua.LFunction
	bell   *lua.LFunction
	resize *lua.LFunction
	focus  *lua.LFunction
	scroll *lua.LFunction
}

// Runtime owns a persistent Lua state and must only be used from the thread that
// created it. The GLFW frontend calls Load, Dispatch, and the Fire* event methods
// on its main loop thread.
type Runtime struct {
	state           *lua.LState
	bindings        []Binding
	events          eventHandlers
	timers          *timerTable
	statuses        *statusTable
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
	// The timer/status tables are shared between the cervterm module closures and
	// the Runtime returned below, so registrations made while the config file
	// runs are already visible before the loop starts.
	tmrs := &timerTable{}
	statuses := &statusTable{}
	state.PreloadModule("cervterm", func(state *lua.LState) int {
		state.Push(buildModule(state, tmrs, statuses))
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
	events, err := loadEvents(root)
	if err != nil {
		state.Close()
		return base, nil, err
	}
	return cfg, &Runtime{state: state, bindings: bindings, events: events, timers: tmrs, statuses: statuses, dispatchTimeout: time.Second}, nil
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
	return r.callProtected("keys "+binding.Spec.String(), binding.fn, host)
}

// WantsOutput reports whether an on-output handler is registered, so the frontend
// can skip converting output chunks when no handler will consume them.
func (r *Runtime) WantsOutput() bool { return r.events.output != nil }

// FireOutput runs the on-output handler with the raw output chunk. It is a no-op
// when no handler is registered.
func (r *Runtime) FireOutput(host Host, data string) error {
	if r.events.output == nil {
		return nil
	}
	return r.callProtected("events.output", r.events.output, host, lua.LString(data))
}

// FireTitle runs the on-title handler with the new title.
func (r *Runtime) FireTitle(host Host, title string) error {
	if r.events.title == nil {
		return nil
	}
	return r.callProtected("events.title", r.events.title, host, lua.LString(title))
}

// FireCwd runs the on-cwd handler with the new working directory.
func (r *Runtime) FireCwd(host Host, dir string) error {
	if r.events.cwd == nil {
		return nil
	}
	return r.callProtected("events.cwd", r.events.cwd, host, lua.LString(dir))
}

// FireBell runs the on-bell handler.
func (r *Runtime) FireBell(host Host) error {
	if r.events.bell == nil {
		return nil
	}
	return r.callProtected("events.bell", r.events.bell, host)
}

// FireResize runs the on-resize handler with the new grid dimensions.
func (r *Runtime) FireResize(host Host, cols, rows int) error {
	if r.events.resize == nil {
		return nil
	}
	return r.callProtected("events.resize", r.events.resize, host, lua.LNumber(cols), lua.LNumber(rows))
}

// FireFocus runs the on-focus handler with the new focus state.
func (r *Runtime) FireFocus(host Host, focused bool) error {
	if r.events.focus == nil {
		return nil
	}
	return r.callProtected("events.focus", r.events.focus, host, lua.LBool(focused))
}

// FireScroll runs the on-scroll handler with the post-clamp viewport offset.
func (r *Runtime) FireScroll(host Host, offset int) error {
	if r.events.scroll == nil {
		return nil
	}
	return r.callProtected("events.scroll", r.events.scroll, host, lua.LNumber(offset))
}

// callProtected invokes fn with a fresh term handle plus extra args under a
// deadline watchdog. A cancelled or erroring call leaves the state reusable.
func (r *Runtime) callProtected(label string, fn *lua.LFunction, host Host, extra ...lua.LValue) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.dispatchTimeout)
	defer cancel()
	r.state.SetContext(ctx)
	defer r.state.SetContext(context.Background())

	args := make([]lua.LValue, 0, len(extra)+1)
	args = append(args, termTable(r.state, host))
	args = append(args, extra...)
	if err := r.state.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, args...); err != nil {
		return fmt.Errorf("%s: %w", label, err)
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

func loadEvents(root *lua.LTable) (eventHandlers, error) {
	var handlers eventHandlers
	value := root.RawGetString("events")
	if value == lua.LNil {
		return handlers, nil
	}
	table, ok := value.(*lua.LTable)
	if !ok {
		return handlers, fmt.Errorf("events must be a table")
	}
	for _, entry := range []struct {
		name string
		dst  **lua.LFunction
	}{
		{"output", &handlers.output},
		{"title", &handlers.title},
		{"cwd", &handlers.cwd},
		{"bell", &handlers.bell},
		{"resize", &handlers.resize},
		{"focus", &handlers.focus},
		{"scroll", &handlers.scroll},
	} {
		value := table.RawGetString(entry.name)
		if value == lua.LNil {
			continue
		}
		fn, ok := value.(*lua.LFunction)
		if !ok {
			return eventHandlers{}, fmt.Errorf("events.%s must be a function", entry.name)
		}
		*entry.dst = fn
	}
	return handlers, nil
}
