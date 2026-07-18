package config

import (
	"fmt"
	"regexp"

	"cervterm/internal/quickselect"
	lua "github.com/yuin/gopher-lua"
)

func validateQuickSelectRuleList(source, path string, value lua.LValue) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindQuickSelectRuleList, value)
	}
	if err := validateDenseArray(source, path, table); err != nil {
		return err
	}
	if table.Len() > quickselect.MaxRules {
		return documentError(source, path, "must contain at most %d entries", quickselect.MaxRules)
	}
	seen := map[string]struct{}{}
	for i := 1; i <= table.Len(); i++ {
		entryPath := fmt.Sprintf("%s[%d]", path, i)
		entry, ok := table.RawGetInt(i).(*lua.LTable)
		if !ok {
			return typeError(source, entryPath, KindTable, table.RawGetInt(i))
		}
		keys, err := strictStringKeys(source, entryPath, entry)
		if err != nil {
			return err
		}
		allowed := map[string]bool{"id": true, "pattern": true, "action": true, "priority": true}
		for _, key := range keys {
			if !allowed[key] {
				return documentError(source, joinPath(entryPath, key), "unknown field")
			}
		}
		id, ok := entry.RawGetString("id").(lua.LString)
		if !ok {
			return typeError(source, entryPath+".id", KindString, entry.RawGetString("id"))
		}
		if len(id) == 0 || len(id) > quickselect.MaxIDBytes {
			return documentError(source, entryPath+".id", "must contain 1 to %d bytes", quickselect.MaxIDBytes)
		}
		if _, dup := seen[string(id)]; dup {
			return documentError(source, entryPath+".id", "duplicate id %q", id)
		}
		seen[string(id)] = struct{}{}
		pattern, ok := entry.RawGetString("pattern").(lua.LString)
		if !ok {
			return typeError(source, entryPath+".pattern", KindString, entry.RawGetString("pattern"))
		}
		if len(pattern) == 0 || len(pattern) > quickselect.MaxPattern {
			return documentError(source, entryPath+".pattern", "must contain 1 to %d bytes", quickselect.MaxPattern)
		}
		if _, err := regexp.Compile(string(pattern)); err != nil {
			return documentError(source, entryPath+".pattern", "invalid regex: %v", err)
		}
		action, ok := entry.RawGetString("action").(lua.LString)
		if !ok {
			return typeError(source, entryPath+".action", KindString, entry.RawGetString("action"))
		}
		if action != "open" && action != "copy" {
			return documentError(source, entryPath+".action", "must be open or copy")
		}
		if v := entry.RawGetString("priority"); v != lua.LNil {
			if err := validateInteger(source, entryPath+".priority", v); err != nil {
				return err
			}
		}
	}
	return nil
}
