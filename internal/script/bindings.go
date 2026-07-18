package script

import (
	"fmt"
	"strings"

	termaction "cervterm/internal/action"

	lua "github.com/yuin/gopher-lua"
)

const (
	MaxBindingTables    = 32
	MaxBindingsPerTable = 128
	MaxKeyBindings      = 512
	MaxMouseBindings    = 128
	MaxBindingDepth     = 4
	MinBindingTimeoutMS = 100
	MaxBindingTimeoutMS = 10000
	MaxBindingCallbacks = 768
)

type CallbackDomain string

const (
	CallbackRoot  CallbackDomain = "root"
	CallbackTable CallbackDomain = "table"
	CallbackMouse CallbackDomain = "mouse"
)

type CallbackRef struct {
	Domain CallbackDomain
	Table  string
	Slot   int
}

type Leader struct {
	Spec      Spec
	TimeoutMS int
}

type Binding struct {
	Spec     Spec
	Action   termaction.Envelope
	Label    string
	ToTable  string
	Callback *CallbackRef
}

type KeyTable struct {
	Name      string
	OneShot   bool
	TimeoutMS int
	Bindings  []Binding
}

type MouseEvent string

type MouseButton string

const (
	MousePress   MouseEvent = "press"
	MouseRelease MouseEvent = "release"
	MouseDrag    MouseEvent = "drag"
	MouseWheel   MouseEvent = "wheel"

	MouseLeft   MouseButton = "left"
	MouseMiddle MouseButton = "middle"
	MouseRight  MouseButton = "right"
	MouseUp     MouseButton = "up"
	MouseDown   MouseButton = "down"
)

type MouseSpec struct {
	Event      MouseEvent
	Button     MouseButton
	Mods       Mod
	ClickCount int
}

type MouseBinding struct {
	Spec     MouseSpec
	Action   termaction.Envelope
	Label    string
	Callback *CallbackRef
}

type BindingSet struct {
	Leader *Leader
	Root   []Binding
	Tables []KeyTable
	Mouse  []MouseBinding
}

func (s BindingSet) Clone() BindingSet {
	out := s
	if s.Leader != nil {
		leader := *s.Leader
		out.Leader = &leader
	}
	out.Root = cloneBindings(s.Root)
	out.Tables = make([]KeyTable, len(s.Tables))
	for i := range s.Tables {
		out.Tables[i] = KeyTable{Name: s.Tables[i].Name, OneShot: s.Tables[i].OneShot, TimeoutMS: s.Tables[i].TimeoutMS, Bindings: cloneBindings(s.Tables[i].Bindings)}
	}
	out.Mouse = append([]MouseBinding(nil), s.Mouse...)
	for i := range out.Mouse {
		out.Mouse[i].Callback = cloneCallbackRef(out.Mouse[i].Callback)
	}
	return out
}

func cloneBindings(in []Binding) []Binding {
	out := append([]Binding(nil), in...)
	for i := range out {
		out[i].Callback = cloneCallbackRef(out[i].Callback)
	}
	return out
}

func cloneCallbackRef(ref *CallbackRef) *CallbackRef {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

type callbackTable map[CallbackRef]*lua.LFunction

type bindingDecoder struct {
	callbacks callbackTable
	count     int
}

func loadBindingSet(root *lua.LTable) (BindingSet, callbackTable, error) {
	d := &bindingDecoder{callbacks: callbackTable{}}
	var set BindingSet
	var err error
	if set.Leader, err = decodeLeader(root.RawGetString("leader")); err != nil {
		return BindingSet{}, nil, err
	}
	if set.Root, err = d.decodeKeys(root.RawGetString("keys"), CallbackRoot, "", "keys"); err != nil {
		return BindingSet{}, nil, err
	}
	if set.Tables, err = d.decodeTables(root.RawGetString("key_tables")); err != nil {
		return BindingSet{}, nil, err
	}
	if set.Mouse, err = d.decodeMouse(root.RawGetString("mouse_bindings")); err != nil {
		return BindingSet{}, nil, err
	}
	if len(set.Root)+tableKeyCount(set.Tables) > MaxKeyBindings {
		return BindingSet{}, nil, fmt.Errorf("bindings: at most %d total keys", MaxKeyBindings)
	}
	if d.count > MaxBindingCallbacks {
		return BindingSet{}, nil, fmt.Errorf("bindings: at most %d callbacks", MaxBindingCallbacks)
	}
	names := map[string]struct{}{}
	for _, table := range set.Tables {
		names[table.Name] = struct{}{}
	}
	for _, binding := range appendAllBindings(set) {
		if binding.ToTable != "" {
			if _, ok := names[binding.ToTable]; !ok {
				return BindingSet{}, nil, fmt.Errorf("binding %s: unknown key table %q", binding.Spec.String(), binding.ToTable)
			}
		}
	}
	if err := validateBindingDepth(set); err != nil {
		return BindingSet{}, nil, err
	}
	return set, d.callbacks, nil
}

func decodeLeader(value lua.LValue) (*Leader, error) {
	if value == lua.LNil {
		return nil, nil
	}
	t, ok := value.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("leader must be a table")
	}
	spec, err := decodeKeySpec(t, "leader")
	if err != nil {
		return nil, err
	}
	timeout := 1000
	if v := t.RawGetString("timeout_ms"); v != lua.LNil {
		n, ok := v.(lua.LNumber)
		if !ok || float64(int(n)) != float64(n) {
			return nil, fmt.Errorf("leader.timeout_ms must be an integer")
		}
		timeout = int(n)
	}
	if timeout < MinBindingTimeoutMS || timeout > MaxBindingTimeoutMS {
		return nil, fmt.Errorf("leader.timeout_ms must be between %d and %d", MinBindingTimeoutMS, MaxBindingTimeoutMS)
	}
	return &Leader{Spec: spec, TimeoutMS: timeout}, nil
}

func (d *bindingDecoder) decodeTables(value lua.LValue) ([]KeyTable, error) {
	if value == lua.LNil {
		return nil, nil
	}
	t, ok := value.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("key_tables must be a table")
	}
	if err := strictArray(t, "key_tables"); err != nil {
		return nil, err
	}
	if t.Len() > MaxBindingTables {
		return nil, fmt.Errorf("key_tables: at most %d tables", MaxBindingTables)
	}
	out := make([]KeyTable, 0, t.Len())
	names := map[string]struct{}{}
	for i := 1; i <= t.Len(); i++ {
		entry, ok := t.RawGetInt(i).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("key_tables[%d] must be a table", i)
		}
		name, ok := entry.RawGetString("name").(lua.LString)
		if !ok || strings.TrimSpace(string(name)) == "" {
			return nil, fmt.Errorf("key_tables[%d].name must be a non-empty string", i)
		}
		n := string(name)
		if _, exists := names[n]; exists {
			return nil, fmt.Errorf("key_tables[%d]: duplicate name %q", i, n)
		}
		names[n] = struct{}{}
		oneShot := true
		if value := entry.RawGetString("one_shot"); value != lua.LNil {
			parsed, ok := value.(lua.LBool)
			if !ok {
				return nil, fmt.Errorf("key_tables[%d].one_shot must be a boolean", i)
			}
			oneShot = bool(parsed)
		}
		timeout := 1500
		if value := entry.RawGetString("timeout_ms"); value != lua.LNil {
			number, ok := value.(lua.LNumber)
			if !ok || float64(int(number)) != float64(number) {
				return nil, fmt.Errorf("key_tables[%d].timeout_ms must be an integer", i)
			}
			timeout = int(number)
			if timeout < MinBindingTimeoutMS || timeout > MaxBindingTimeoutMS {
				return nil, fmt.Errorf("key_tables[%d].timeout_ms must be between %d and %d", i, MinBindingTimeoutMS, MaxBindingTimeoutMS)
			}
		}
		keys, err := d.decodeKeys(entry.RawGetString("keys"), CallbackTable, n, fmt.Sprintf("key_tables[%d].keys", i))
		if err != nil {
			return nil, err
		}
		out = append(out, KeyTable{Name: n, OneShot: oneShot, TimeoutMS: timeout, Bindings: keys})
	}
	return out, nil
}

func (d *bindingDecoder) decodeKeys(value lua.LValue, domain CallbackDomain, table, path string) ([]Binding, error) {
	if value == lua.LNil {
		return nil, nil
	}
	t, ok := value.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("%s must be a table", path)
	}
	if err := strictArray(t, path); err != nil {
		return nil, err
	}
	if t.Len() > MaxBindingsPerTable {
		return nil, fmt.Errorf("%s: at most %d bindings", path, MaxBindingsPerTable)
	}
	out := make([]Binding, 0, t.Len())
	seen := map[Spec]struct{}{}
	for i := 1; i <= t.Len(); i++ {
		entry, ok := t.RawGetInt(i).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: entry must be a table", path, i)
		}
		spec, err := decodeKeySpec(entry, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		if _, exists := seen[spec]; exists {
			return nil, fmt.Errorf("%s[%d]: duplicate key spec %q", path, i, spec.String())
		}
		seen[spec] = struct{}{}
		binding := Binding{Spec: spec}
		if v := entry.RawGetString("label"); v != lua.LNil {
			label, ok := v.(lua.LString)
			if !ok {
				return nil, fmt.Errorf("%s[%d].label must be a string", path, i)
			}
			binding.Label = string(label)
		}
		if v := entry.RawGetString("table"); v != lua.LNil {
			name, ok := v.(lua.LString)
			if !ok || name == "" {
				return nil, fmt.Errorf("%s[%d].table must be a non-empty string", path, i)
			}
			binding.ToTable = string(name)
		}
		action := entry.RawGetString("action")
		if binding.ToTable != "" && action != lua.LNil {
			return nil, fmt.Errorf("%s[%d]: action and table are mutually exclusive", path, i)
		}
		if binding.ToTable == "" {
			ref := CallbackRef{Domain: domain, Table: table, Slot: i - 1}
			if err := d.decodeAction(action, &binding.Action, &binding.Callback, ref, binding.Label, path, i); err != nil {
				return nil, err
			}
		}
		out = append(out, binding)
	}
	return out, nil
}

func (d *bindingDecoder) decodeMouse(value lua.LValue) ([]MouseBinding, error) {
	if value == lua.LNil {
		return nil, nil
	}
	t, ok := value.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("mouse_bindings must be a table")
	}
	if err := strictArray(t, "mouse_bindings"); err != nil {
		return nil, err
	}
	if t.Len() > MaxMouseBindings {
		return nil, fmt.Errorf("mouse_bindings: at most %d bindings", MaxMouseBindings)
	}
	out := make([]MouseBinding, 0, t.Len())
	seen := map[MouseSpec]struct{}{}
	for i := 1; i <= t.Len(); i++ {
		entry, ok := t.RawGetInt(i).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("mouse_bindings[%d] must be a table", i)
		}
		spec, err := decodeMouseSpec(entry, i)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[spec]; exists {
			return nil, fmt.Errorf("mouse_bindings[%d]: duplicate mouse spec", i)
		}
		seen[spec] = struct{}{}
		binding := MouseBinding{Spec: spec}
		if v := entry.RawGetString("label"); v != lua.LNil {
			label, ok := v.(lua.LString)
			if !ok {
				return nil, fmt.Errorf("mouse_bindings[%d].label must be a string", i)
			}
			binding.Label = string(label)
		}
		ref := CallbackRef{Domain: CallbackMouse, Slot: i - 1}
		if err := d.decodeAction(entry.RawGetString("action"), &binding.Action, &binding.Callback, ref, binding.Label, "mouse_bindings", i); err != nil {
			return nil, err
		}
		out = append(out, binding)
	}
	return out, nil
}

func (d *bindingDecoder) decodeAction(value lua.LValue, action *termaction.Envelope, callback **CallbackRef, ref CallbackRef, label, path string, index int) error {
	var err error
	switch value := value.(type) {
	case *lua.LFunction:
		d.count++
		if d.count > MaxBindingCallbacks {
			return fmt.Errorf("bindings: at most %d callbacks", MaxBindingCallbacks)
		}
		d.callbacks[ref] = value
		*callback = cloneCallbackRef(&ref)
		*action, err = termaction.New(termaction.Callback{BindingIndex: ref.Slot, Label: label}, termaction.TargetFocused)
	case *lua.LUserData:
		var ok bool
		*action, ok = luaAction(value)
		if !ok {
			return fmt.Errorf("%s[%d]: action userdata is not a cervterm action", path, index)
		}
	default:
		return fmt.Errorf("%s[%d]: action must be a function or cervterm action", path, index)
	}
	if err != nil {
		return fmt.Errorf("%s[%d]: %w", path, index, err)
	}
	if err := action.Validate(); err != nil {
		return fmt.Errorf("%s[%d]: invalid action: %w", path, index, err)
	}
	return nil
}

func decodeKeySpec(t *lua.LTable, path string) (Spec, error) {
	key, ok := t.RawGetString("key").(lua.LString)
	if !ok {
		return Spec{}, fmt.Errorf("%s: key must be a string", path)
	}
	mods := ""
	if v := t.RawGetString("mods"); v != lua.LNil {
		s, ok := v.(lua.LString)
		if !ok {
			return Spec{}, fmt.Errorf("%s: mods must be a string", path)
		}
		mods = string(s)
	}
	spec, err := ParseSpec(string(key), mods)
	if err != nil {
		return Spec{}, fmt.Errorf("%s: %w", path, err)
	}
	return spec, nil
}

func decodeMouseSpec(t *lua.LTable, i int) (MouseSpec, error) {
	path := fmt.Sprintf("mouse_bindings[%d]", i)
	event, ok := t.RawGetString("event").(lua.LString)
	if !ok {
		return MouseSpec{}, fmt.Errorf("%s.event must be a string", path)
	}
	button, ok := t.RawGetString("button").(lua.LString)
	if !ok {
		return MouseSpec{}, fmt.Errorf("%s.button must be a string", path)
	}
	s := MouseSpec{Event: MouseEvent(strings.ToLower(string(event))), Button: MouseButton(strings.ToLower(string(button))), ClickCount: 1}
	if s.Event != MousePress && s.Event != MouseRelease && s.Event != MouseDrag && s.Event != MouseWheel {
		return MouseSpec{}, fmt.Errorf("%s.event is invalid", path)
	}
	if s.Button != MouseLeft && s.Button != MouseMiddle && s.Button != MouseRight && s.Button != MouseUp && s.Button != MouseDown {
		return MouseSpec{}, fmt.Errorf("%s.button is invalid", path)
	}
	if s.Event == MouseWheel && s.Button != MouseUp && s.Button != MouseDown {
		return MouseSpec{}, fmt.Errorf("%s: wheel requires up or down button", path)
	}
	if s.Event != MouseWheel && (s.Button == MouseUp || s.Button == MouseDown) {
		return MouseSpec{}, fmt.Errorf("%s: up/down buttons require wheel event", path)
	}
	if v := t.RawGetString("mods"); v != lua.LNil {
		mods, ok := v.(lua.LString)
		if !ok {
			return MouseSpec{}, fmt.Errorf("%s.mods must be a string", path)
		}
		parsed, err := parseMods(string(mods))
		if err != nil {
			return MouseSpec{}, fmt.Errorf("%s: %w", path, err)
		}
		s.Mods = parsed
	}
	if v := t.RawGetString("click_count"); v != lua.LNil {
		n, ok := v.(lua.LNumber)
		if !ok || n < 1 || n > 3 || float64(int(n)) != float64(n) {
			return MouseSpec{}, fmt.Errorf("%s.click_count must be an integer from 1 to 3", path)
		}
		s.ClickCount = int(n)
	}
	return s, nil
}

func parseMods(value string) (Mod, error) {
	spec, err := ParseSpec("a", value)
	if err != nil {
		return 0, err
	}
	return spec.Mods, nil
}
