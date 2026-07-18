package config

import (
	"sort"

	lua "github.com/yuin/gopher-lua"
)

func (b *compositionBuilder) mergeFeatureMap(dst *lua.LTable, schema fieldSchema, path string, value lua.LValue) error {
	source, ok := value.(*lua.LTable)
	if !ok {
		return nil
	}
	target, ok := dst.RawGetString(schema.name).(*lua.LTable)
	if !ok {
		target = b.state.NewTable()
		dst.RawSetString(schema.name, target)
	}
	keys := make([]string, 0, source.Len())
	source.ForEach(func(key, _ lua.LValue) {
		if parsed, ok := key.(lua.LString); ok {
			keys = append(keys, string(parsed))
		}
	})
	sort.Strings(keys)
	for _, key := range keys {
		entry := source.RawGetString(key)
		entryPath := mapEntryPath(path, key)
		if isUnsetValue(entry) && b.document.AuthoredVersion == 2 {
			if err := b.consume(1, entryPath); err != nil {
				return err
			}
			target.RawSetString(key, lua.LNil)
			b.provenance.set(entryPath, b.origin, true, schema.sensitive)
			continue
		}
		if _, ok := entry.(lua.LNumber); !ok {
			continue
		}
		if err := b.consume(1, entryPath); err != nil {
			return err
		}
		target.RawSetString(key, entry)
		b.provenance.set(entryPath, b.origin, false, schema.sensitive)
	}
	return nil
}
