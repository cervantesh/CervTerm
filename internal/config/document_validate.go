package config

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	termaction "cervterm/internal/action"

	lua "github.com/yuin/gopher-lua"
)

func validateStrictTable(source, path string, table *lua.LTable, schema fieldSchema, root bool) error {
	allowed := make(map[string]fieldSchema, len(schema.children)+1)
	for _, child := range schema.children {
		allowed[child.name] = child
	}
	if root {
		allowed["config_version"] = fieldSchema{name: "config_version", kind: KindInteger}
		for name := range unavailableV2Fields {
			allowed[name] = fieldSchema{name: name, kind: KindTable}
		}
	}
	keys, err := strictStringKeys(source, path, table)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if _, ok := allowed[key]; !ok {
			return documentError(source, joinPath(path, key), "unknown field")
		}
	}
	for _, child := range schema.children {
		value := table.RawGetString(child.name)
		if value == lua.LNil {
			if child.required {
				return documentError(source, joinPath(path, child.name), "is required")
			}
			continue
		}
		if err := validateStrictValue(source, joinPath(path, child.name), value, child); err != nil {
			return err
		}
	}
	return nil
}

func validateStrictValue(source, path string, value lua.LValue, schema fieldSchema) error {
	switch schema.kind {
	case KindTable:
		table, ok := value.(*lua.LTable)
		if !ok {
			return typeError(source, path, KindTable, value)
		}
		return validateStrictTable(source, path, table, schema, false)
	case KindString:
		if _, ok := value.(lua.LString); !ok {
			return typeError(source, path, KindString, value)
		}
	case KindBoolean:
		if _, ok := value.(lua.LBool); !ok {
			return typeError(source, path, KindBoolean, value)
		}
	case KindNumber:
		number, ok := value.(lua.LNumber)
		if !ok {
			return typeError(source, path, KindNumber, value)
		}
		if parsed := float64(number); math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return documentError(source, path, "must be finite")
		}
	case KindInteger:
		if err := validateInteger(source, path, value); err != nil {
			return err
		}
	case KindStringList:
		return validateStringList(source, path, value)
	case KindStringMap:
		return validateStringMap(source, path, value)
	case KindKeyList:
		return validateKeyList(source, path, value)
	case KindEvents:
		return validateEvents(source, path, value)
	default:
		return documentError(source, path, "has unsupported schema kind %q", schema.kind)
	}
	return nil
}

func validateInteger(source, path string, value lua.LValue) error {
	number, ok := value.(lua.LNumber)
	if !ok {
		return typeError(source, path, KindInteger, value)
	}
	parsed := float64(number)
	upper := math.Ldexp(1, strconv.IntSize-1)
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) || math.Trunc(parsed) != parsed || parsed < -upper || parsed >= upper {
		return documentError(source, path, "must be an integer in [%g, %g)", -upper, upper)
	}
	return nil
}

func validateStringList(source, path string, value lua.LValue) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindStringList, value)
	}
	if err := validateDenseArray(source, path, table); err != nil {
		return err
	}
	for i := 1; i <= table.Len(); i++ {
		if _, ok := table.RawGetInt(i).(lua.LString); !ok {
			return typeError(source, fmt.Sprintf("%s[%d]", path, i), KindString, table.RawGetInt(i))
		}
	}
	return nil
}

func validateStringMap(source, path string, value lua.LValue) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindStringMap, value)
	}
	var failures []string
	table.ForEach(func(key, value lua.LValue) {
		stringKey, keyOK := key.(lua.LString)
		_, valueOK := value.(lua.LString)
		if !keyOK {
			failures = append(failures, fmt.Sprintf("%s: map key must be string, got %s", path, key.Type().String()))
		} else if !valueOK {
			failures = append(failures, fmt.Sprintf("%s.%s: must be string, got %s", path, string(stringKey), value.Type().String()))
		}
	})
	if len(failures) > 0 {
		sort.Strings(failures)
		return fmt.Errorf("%s: %s", sourceLabel(source), failures[0])
	}
	return nil
}

func validateKeyList(source, path string, value lua.LValue) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindKeyList, value)
	}
	if err := validateDenseArray(source, path, table); err != nil {
		return err
	}
	for i := 1; i <= table.Len(); i++ {
		entryPath := fmt.Sprintf("%s[%d]", path, i)
		entry, ok := table.RawGetInt(i).(*lua.LTable)
		if !ok {
			return typeError(source, entryPath, KindTable, table.RawGetInt(i))
		}
		allowed := map[string]struct{}{"key": {}, "mods": {}, "label": {}, "action": {}}
		keys, err := strictStringKeys(source, entryPath, entry)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if _, ok := allowed[key]; !ok {
				return documentError(source, entryPath+"."+key, "unknown field")
			}
		}
		if _, ok := entry.RawGetString("key").(lua.LString); !ok {
			return documentError(source, entryPath+".key", "must be a string")
		}
		if mods := entry.RawGetString("mods"); mods != lua.LNil {
			if _, ok := mods.(lua.LString); !ok {
				return typeError(source, entryPath+".mods", KindString, mods)
			}
		}
		if label := entry.RawGetString("label"); label != lua.LNil {
			if _, ok := label.(lua.LString); !ok {
				return typeError(source, entryPath+".label", KindString, label)
			}
		}
		actionValue := entry.RawGetString("action")
		switch action := actionValue.(type) {
		case *lua.LFunction:
		case *lua.LUserData:
			envelope, ok := action.Value.(termaction.Envelope)
			if !ok {
				return documentError(source, entryPath+".action", "userdata is not a cervterm action")
			}
			if err := envelope.Validate(); err != nil {
				return documentError(source, entryPath+".action", "invalid cervterm action: %v", err)
			}
		default:
			return documentError(source, entryPath+".action", "must be a function or cervterm action")
		}
	}
	return nil
}

func validateEvents(source, path string, value lua.LValue) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindEvents, value)
	}
	allowed := map[string]struct{}{"output": {}, "title": {}, "cwd": {}, "bell": {}, "resize": {}, "focus": {}, "scroll": {}}
	keys, err := strictStringKeys(source, path, table)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if _, ok := allowed[key]; !ok {
			return documentError(source, path+"."+key, "unknown field")
		}
		if value := table.RawGetString(key); value != lua.LNil && value.Type() != lua.LTFunction {
			return documentError(source, path+"."+key, "must be a function")
		}
	}
	return nil
}

func validateDenseArray(source, path string, table *lua.LTable) error {
	length := table.Len()
	seen := make(map[int]struct{}, length)
	var invalid []string
	table.ForEach(func(key, _ lua.LValue) {
		number, ok := key.(lua.LNumber)
		if !ok {
			invalid = append(invalid, "type "+key.Type().String())
			return
		}
		parsed := float64(number)
		index := int(parsed)
		if parsed != float64(index) || index < 1 || index > length {
			invalid = append(invalid, fmt.Sprintf("%g", parsed))
			return
		}
		seen[index] = struct{}{}
	})
	if len(invalid) > 0 {
		sort.Strings(invalid)
		return documentError(source, path, "must be a dense 1-based array; invalid key %q", invalid[0])
	}
	for i := 1; i <= length; i++ {
		if _, ok := seen[i]; !ok || table.RawGetInt(i) == lua.LNil {
			return documentError(source, path, "must be a dense 1-based array; missing index %d", i)
		}
	}
	return nil
}

func strictStringKeys(source, path string, table *lua.LTable) ([]string, error) {
	keys := make([]string, 0, table.Len())
	invalidTypes := make([]string, 0, 1)
	table.ForEach(func(key, _ lua.LValue) {
		if stringKey, ok := key.(lua.LString); ok {
			keys = append(keys, string(stringKey))
		} else {
			invalidTypes = append(invalidTypes, key.Type().String())
		}
	})
	if len(invalidTypes) > 0 {
		sort.Strings(invalidTypes)
		if path == "" {
			path = "root"
		}
		return nil, documentError(source, path, "field names must be strings, got %s key", invalidTypes[0])
	}
	sort.Strings(keys)
	return keys, nil
}

func typeError(source, path string, want ValueKind, got lua.LValue) error {
	return documentError(source, path, "must be %s, got %s", strings.ReplaceAll(string(want), "_", " "), got.Type().String())
}

func documentError(source, path, format string, args ...any) error {
	return fmt.Errorf("%s: %s: %s", sourceLabel(source), path, fmt.Sprintf(format, args...))
}

func sourceLabel(source string) string {
	if strings.TrimSpace(source) == "" {
		return "config"
	}
	return source
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
