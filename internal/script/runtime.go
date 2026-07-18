package script

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cervterm/internal/config"

	lua "github.com/yuin/gopher-lua"
)

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
	bindings        BindingSet
	callbacks       callbackTable
	events          eventHandlers
	timers          *timerTable
	statuses        *statusTable
	overlays        *overlayStore
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
	overlays := &overlayStore{}
	state.PreloadModule("cervterm", func(state *lua.LState) int {
		state.Push(buildModule(state, tmrs, statuses, overlays))
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
	document, err := config.DecodeDocument(path, root)
	if err != nil {
		state.Close()
		return base, nil, err
	}
	cfg := config.FromDocument(base, document)
	bindings, callbacks, err := loadBindingSet(root)
	if err != nil {
		state.Close()
		return base, nil, err
	}
	events, err := loadEvents(root)
	if err != nil {
		state.Close()
		return base, nil, err
	}
	return cfg, &Runtime{state: state, bindings: bindings, callbacks: callbacks, events: events, timers: tmrs, statuses: statuses, overlays: overlays, dispatchTimeout: time.Second}, nil
}

// BindingSet returns a detached snapshot of all decoded input bindings.
func (r *Runtime) BindingSet() BindingSet {
	if r == nil {
		return BindingSet{}
	}
	return r.bindings.Clone()
}

// Bindings preserves the legacy flat-root adapter.
func (r *Runtime) Bindings() []Binding {
	if r == nil {
		return nil
	}
	return cloneBindings(r.bindings.Root)
}

func (r *Runtime) Dispatch(index int, host Host) error {
	if index < 0 || index >= len(r.bindings.Root) {
		return fmt.Errorf("binding index %d out of range", index)
	}
	binding := r.bindings.Root[index]
	if binding.Callback == nil {
		return fmt.Errorf("binding %d action %q is not a Lua callback", index, binding.Action.Action.ID())
	}
	fn := r.callbacks[*binding.Callback]
	if fn == nil {
		return fmt.Errorf("binding %d callback reference is not registered", index)
	}
	return r.callProtected("keys "+binding.Spec.String(), fn, host)
}

// DispatchRef invokes a runtime-local callback from any binding domain.
func (r *Runtime) DispatchRef(ref CallbackRef, label string, host Host) error {
	if r == nil {
		return fmt.Errorf("runtime is unavailable")
	}
	fn := r.callbacks[ref]
	if fn == nil {
		return fmt.Errorf("callback %s/%s/%d is not registered", ref.Domain, ref.Table, ref.Slot)
	}
	if label == "" {
		label = fmt.Sprintf("%s/%s[%d]", ref.Domain, ref.Table, ref.Slot)
	}
	return r.callProtected(label, fn, host)
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
