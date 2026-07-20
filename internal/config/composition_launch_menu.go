package config

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func (b *compositionBuilder) mergeLaunchTargets(dst *lua.LTable, schema fieldSchema, path string, value lua.LValue) error {
	list, ok := value.(*lua.LTable)
	if !ok {
		return nil
	}
	if err := b.consume(1+list.Len()*8, path); err != nil {
		return err
	}
	dst.RawSetString(schema.name, list)
	b.provenance.tombstonePrefixExcept(path, b.origin, false, nil)
	b.provenance.set(path, b.origin, false, false)
	for i := 1; i <= list.Len(); i++ {
		entry, ok := list.RawGetInt(i).(*lua.LTable)
		if !ok {
			continue
		}
		base := fmt.Sprintf("%s[%d]", path, i)
		for _, field := range []string{"id", "label", "program", "cwd"} {
			if entry.RawGetString(field) != lua.LNil {
				b.provenance.set(base+"."+field, b.origin, false, false)
			}
		}
		if args, ok := entry.RawGetString("args").(*lua.LTable); ok {
			for j := 1; j <= args.Len(); j++ {
				b.provenance.set(fmt.Sprintf("%s.args[%d]", base, j), b.origin, false, false)
			}
		}
		if env, ok := entry.RawGetString("env").(*lua.LTable); ok {
			env.ForEach(func(key, _ lua.LValue) {
				if name, ok := key.(lua.LString); ok {
					b.provenance.set(mapEntryPath(base+".env", string(name)), b.origin, false, true)
				}
			})
		}
	}
	return nil
}
