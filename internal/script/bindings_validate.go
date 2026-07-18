package script

import (
	"fmt"
	"sort"

	lua "github.com/yuin/gopher-lua"
)

func strictArray(t *lua.LTable, path string) error {
	length, count := t.Len(), 0
	var bad string
	t.ForEach(func(k, _ lua.LValue) {
		count++
		n, ok := k.(lua.LNumber)
		if !ok || n < 1 || float64(int(n)) != float64(n) || int(n) > length {
			bad = k.String()
		}
	})
	if bad != "" || count != length {
		return fmt.Errorf("%s must be a contiguous array", path)
	}
	return nil
}

func tableKeyCount(tables []KeyTable) int {
	n := 0
	for _, table := range tables {
		n += len(table.Bindings)
	}
	return n
}

func appendAllBindings(set BindingSet) []Binding {
	out := append([]Binding(nil), set.Root...)
	for _, table := range set.Tables {
		out = append(out, table.Bindings...)
	}
	return out
}

func validateBindingDepth(set BindingSet) error {
	tables := make(map[string]KeyTable, len(set.Tables))
	for _, table := range set.Tables {
		tables[table.Name] = table
	}
	var visit func(string, int, map[string]bool) error
	visit = func(name string, depth int, active map[string]bool) error {
		if depth > MaxBindingDepth {
			return fmt.Errorf("key table %q exceeds maximum chord depth %d", name, MaxBindingDepth)
		}
		if active[name] {
			return fmt.Errorf("key table transition cycle at %q", name)
		}
		active[name] = true
		defer delete(active, name)
		for _, binding := range tables[name].Bindings {
			if binding.ToTable != "" {
				if err := visit(binding.ToTable, depth+1, active); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, binding := range set.Root {
		if binding.ToTable != "" {
			if err := visit(binding.ToTable, 1, map[string]bool{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s BindingSet) Table(name string) (KeyTable, bool) {
	i := sort.Search(len(s.Tables), func(i int) bool { return s.Tables[i].Name >= name })
	if i < len(s.Tables) && s.Tables[i].Name == name {
		t := s.Tables[i]
		t.Bindings = cloneBindings(t.Bindings)
		return t, true
	}
	for _, t := range s.Tables {
		if t.Name == name {
			t.Bindings = cloneBindings(t.Bindings)
			return t, true
		}
	}
	return KeyTable{}, false
}
