package config

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func validateLaunchTargetList(source, path string, value lua.LValue) error {
	list, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindLaunchTargetList, value)
	}
	if err := validateDenseArray(source, path, list); err != nil {
		return err
	}
	if list.Len() > MaxLaunchTargets {
		return documentError(source, path, "must contain at most %d entries", MaxLaunchTargets)
	}
	seen := map[string]struct{}{}
	for i := 1; i <= list.Len(); i++ {
		entryPath := fmt.Sprintf("%s[%d]", path, i)
		entry, ok := list.RawGetInt(i).(*lua.LTable)
		if !ok {
			return typeError(source, entryPath, KindTable, list.RawGetInt(i))
		}
		keys, err := strictStringKeys(source, entryPath, entry)
		if err != nil {
			return err
		}
		allowed := map[string]bool{"id": true, "label": true, "program": true, "args": true, "cwd": true, "env": true}
		for _, key := range keys {
			if !allowed[key] {
				return documentError(source, joinPath(entryPath, key), "unknown field")
			}
		}
		for _, field := range []struct {
			name     string
			min, max int
		}{{"id", 1, MaxLaunchIDBytes}, {"label", 1, MaxLaunchLabelBytes}, {"program", 1, MaxLaunchValueBytes}} {
			v, ok := entry.RawGetString(field.name).(lua.LString)
			if !ok {
				return typeError(source, entryPath+"."+field.name, KindString, entry.RawGetString(field.name))
			}
			if err := launchString(entryPath+"."+field.name, string(v), field.min, field.max); err != nil {
				return documentError(source, entryPath+"."+field.name, "%v", err)
			}
		}
		id := string(entry.RawGetString("id").(lua.LString))
		if _, dup := seen[id]; dup {
			return documentError(source, entryPath+".id", "duplicate id %q", id)
		}
		seen[id] = struct{}{}
		if v := entry.RawGetString("cwd"); v != lua.LNil {
			s, ok := v.(lua.LString)
			if !ok {
				return typeError(source, entryPath+".cwd", KindString, v)
			}
			if err := launchString(entryPath+".cwd", string(s), 0, MaxLaunchValueBytes); err != nil {
				return documentError(source, entryPath+".cwd", "%v", err)
			}
		}
		if v := entry.RawGetString("args"); v != lua.LNil {
			args, ok := v.(*lua.LTable)
			if !ok {
				return typeError(source, entryPath+".args", KindStringList, v)
			}
			if err := validateDenseArray(source, entryPath+".args", args); err != nil {
				return err
			}
			if args.Len() > MaxLaunchArgs {
				return documentError(source, entryPath+".args", "must contain at most %d entries", MaxLaunchArgs)
			}
			for j := 1; j <= args.Len(); j++ {
				s, ok := args.RawGetInt(j).(lua.LString)
				if !ok {
					return typeError(source, fmt.Sprintf("%s.args[%d]", entryPath, j), KindString, args.RawGetInt(j))
				}
				if err := launchString(fmt.Sprintf("%s.args[%d]", entryPath, j), string(s), 0, MaxLaunchValueBytes); err != nil {
					return documentError(source, fmt.Sprintf("%s.args[%d]", entryPath, j), "%v", err)
				}
			}
		}
		if v := entry.RawGetString("env"); v != lua.LNil {
			env, ok := v.(*lua.LTable)
			if !ok {
				return typeError(source, entryPath+".env", KindStringMap, v)
			}
			count := 0
			var validation error
			env.ForEach(func(k, val lua.LValue) {
				if validation != nil {
					return
				}
				count++
				ks, kok := k.(lua.LString)
				vs, vok := val.(lua.LString)
				if !kok || !vok {
					validation = documentError(source, entryPath+".env", "keys and values must be strings")
					return
				}
				if err := launchString(entryPath+".env key", string(ks), 1, MaxLaunchValueBytes); err != nil {
					validation = err
					return
				}
				validation = launchString(entryPath+".env["+string(ks)+"]", string(vs), 0, MaxLaunchValueBytes)
			})
			if validation != nil {
				return documentError(source, entryPath+".env", "%v", validation)
			}
			if count > MaxLaunchEnv {
				return documentError(source, entryPath+".env", "must contain at most %d entries", MaxLaunchEnv)
			}
		}
	}
	return nil
}
