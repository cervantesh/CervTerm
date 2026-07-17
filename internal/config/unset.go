package config

import (
	"fmt"
	"sort"

	lua "github.com/yuin/gopher-lua"
)

var unsetToken = &struct{ name string }{name: "cervterm.config.unset"}

// NewUnsetValue creates the immutable sentinel exposed as cervterm.config.unset.
// The sentinel is recognized only by candidate composition; public single-source
// loading continues to reject it until composed bundles are activated.
func NewUnsetValue(state *lua.LState) *lua.LUserData {
	value := state.NewUserData()
	value.Value = unsetToken
	return value
}

func isUnsetValue(value lua.LValue) bool {
	userdata, ok := value.(*lua.LUserData)
	return ok && userdata.Value == unsetToken
}

func findUnsetPath(table *lua.LTable, schema fieldSchema, prefix string) string {
	for _, child := range schema.children {
		value := table.RawGetString(child.name)
		if value == lua.LNil {
			continue
		}
		path := joinPath(prefix, child.name)
		if isUnsetValue(value) {
			return path
		}
		switch child.kind {
		case KindTable:
			if nested, ok := value.(*lua.LTable); ok {
				if found := findUnsetPath(nested, child, path); found != "" {
					return found
				}
			}
		case KindStringMap:
			if nested, ok := value.(*lua.LTable); ok {
				var found []string
				nested.ForEach(func(key, entry lua.LValue) {
					if isUnsetValue(entry) {
						found = append(found, fmt.Sprintf("%s[%q]", path, key.String()))
					}
				})
				sort.Strings(found)
				if len(found) > 0 {
					return found[0]
				}
			}
		case KindIndexedColorMap:
			if nested, ok := value.(*lua.LTable); ok {
				for _, key := range indexedColorKeys(nested) {
					if isUnsetValue(nested.RawGetInt(key)) {
						return indexedColorEntryPath(path, key)
					}
				}
			}
		case KindEvents:
			if nested, ok := value.(*lua.LTable); ok {
				for _, name := range eventFieldNames {
					if isUnsetValue(nested.RawGetString(name)) {
						return joinPath(path, name)
					}
				}
			}
		case KindStringList:
			if nested, ok := value.(*lua.LTable); ok {
				for i := 1; i <= nested.Len(); i++ {
					if isUnsetValue(nested.RawGetInt(i)) {
						return fmt.Sprintf("%s[%d]", path, i)
					}
				}
			}
		case KindKeyList:
			if nested, ok := value.(*lua.LTable); ok {
				for i := 1; i <= nested.Len(); i++ {
					entry := nested.RawGetInt(i)
					if isUnsetValue(entry) {
						return fmt.Sprintf("%s[%d]", path, i)
					}
					if binding, ok := entry.(*lua.LTable); ok && isUnsetValue(binding.RawGetString("action")) {
						return fmt.Sprintf("%s[%d].action", path, i)
					}
				}
			}
		}
	}
	return ""
}
