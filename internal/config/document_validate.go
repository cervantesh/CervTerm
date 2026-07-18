package config

import (
	"fmt"
	"math"
	"sort"
	"strings"

	termaction "cervterm/internal/action"
	"cervterm/internal/fontdesc"

	lua "github.com/yuin/gopher-lua"
)

func validateStrictTable(source, path string, table *lua.LTable, schema fieldSchema, root bool, extras map[string]fieldSchema, allowUnset bool) error {
	allowed := make(map[string]fieldSchema, len(schema.children)+1)
	for _, child := range schema.children {
		allowed[child.name] = child
	}
	if root {
		allowed["config_version"] = fieldSchema{name: "config_version", kind: KindInteger}
		for name := range unavailableV2Fields {
			allowed[name] = fieldSchema{name: name, kind: KindTable}
		}
		for name, extra := range extras {
			allowed[name] = extra
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
		if err := validateStrictValue(source, joinPath(path, child.name), value, child, allowUnset); err != nil {
			return err
		}
	}
	if root {
		names := make([]string, 0, len(extras))
		for name := range extras {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			value := table.RawGetString(name)
			if value != lua.LNil {
				if err := validateStrictValue(source, name, value, extras[name], false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateStrictValue(source, path string, value lua.LValue, schema fieldSchema, allowUnset bool) error {
	if isUnsetValue(value) {
		if allowUnset {
			return nil
		}
		return documentError(source, path, "cervterm.config.unset is not allowed at this field")
	}
	switch schema.kind {
	case KindTable:
		table, ok := value.(*lua.LTable)
		if !ok {
			return typeError(source, path, KindTable, value)
		}
		return validateStrictTable(source, path, table, schema, false, nil, allowUnset)
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
		parsed := float64(number)
		if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return documentError(source, path, "must be finite")
		}
		if err := validateFontMetricNumber(source, path, parsed); err != nil {
			return err
		}
	case KindInteger:
		if err := validateInteger(source, path, value); err != nil {
			return err
		}
		if isWindowSidePaddingPath(path) {
			parsed := int(value.(lua.LNumber))
			if parsed < 0 || parsed > MaxWindowPadding {
				return documentError(source, path, "must be between 0 and %d", MaxWindowPadding)
			}
		}
	case KindStringList:
		if schema.name == "ansi" {
			return validateANSIList(source, path, value)
		}
		return validateStringList(source, path, value)
	case KindDescriptorList:
		return validateDescriptorList(source, path, value)
	case KindFontRuleList:
		return validateFontRuleList(source, path, value)
	case KindQuickSelectRuleList:
		return validateQuickSelectRuleList(source, path, value)
	case KindStringMap:
		return validateStringMap(source, path, value, allowUnset)
	case KindFeatureMap:
		return validateFeatureMap(source, path, value, allowUnset)
	case KindIndexedColorMap:
		return validateIndexedColorMap(source, path, value, allowUnset)
	case KindKeyList:
		return validateKeyList(source, path, value)
	case KindEvents:
		return validateEvents(source, path, value, allowUnset)
	case KindDocumentMap:
		return validateDocumentMap(source, path, value)
	case KindColorSchemeMap:
		return validateColorSchemeMap(source, path, value, allowUnset)
	case KindBackgroundLayerList:
		return validateBackgroundLayerList(source, path, value)
	default:
		return documentError(source, path, "has unsupported schema kind %q", schema.kind)
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

func validateANSIList(source, path string, value lua.LValue) error {
	if err := validateStringList(source, path, value); err != nil {
		return err
	}
	table := value.(*lua.LTable)
	if table.Len() != 16 {
		return documentError(source, path, "must contain exactly 16 entries, got %d", table.Len())
	}
	for index := 1; index <= table.Len(); index++ {
		color := string(table.RawGetInt(index).(lua.LString))
		if !isHexRGBColor(color) {
			return documentError(source, fmt.Sprintf("%s[%d]", path, index), "must be #RRGGBB")
		}
	}
	return nil
}

func validateIndexedColorMap(source, path string, value lua.LValue, allowUnset bool) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindIndexedColorMap, value)
	}
	var failures []string
	table.ForEach(func(key, value lua.LValue) {
		number, keyOK := key.(lua.LNumber)
		parsed := float64(number)
		if !keyOK || math.IsNaN(parsed) || math.IsInf(parsed, 0) || math.Trunc(parsed) != parsed || parsed < firstIndexedColor || parsed > 255 {
			failures = append(failures, fmt.Sprintf("%s: map key %q must be an integer between 16 and 255", path, key.String()))
			return
		}
		entryPath := fmt.Sprintf("%s[%d]", path, int(parsed))
		if allowUnset && isUnsetValue(value) {
			return
		}
		color, valueOK := value.(lua.LString)
		if !valueOK {
			failures = append(failures, fmt.Sprintf("%s: must be string%s, got %s", entryPath, unsetExpectation(allowUnset), value.Type().String()))
			return
		}
		if !isHexRGBColor(string(color)) {
			failures = append(failures, fmt.Sprintf("%s: must be #RRGGBB", entryPath))
		}
	})
	if len(failures) != 0 {
		sort.Strings(failures)
		return fmt.Errorf("%s: %s", sourceLabel(source), failures[0])
	}
	return nil
}

func unsetExpectation(allowUnset bool) string {
	if allowUnset {
		return " or cervterm.config.unset"
	}
	return ""
}

func validateFeatureMap(source, path string, value lua.LValue, allowUnset bool) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindFeatureMap, value)
	}
	var failures []string
	concrete := 0
	table.ForEach(func(key, value lua.LValue) {
		name, keyOK := key.(lua.LString)
		if !keyOK {
			failures = append(failures, fmt.Sprintf("%s: map key must be a 4-byte ASCII string, got %s", path, key.Type().String()))
			return
		}
		entryPath := mapEntryPath(path, string(name))
		if err := fontdesc.ValidateFeatureTag(string(name)); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", entryPath, err))
			return
		}
		if allowUnset && isUnsetValue(value) {
			return
		}
		if err := validateInteger(source, entryPath, value); err != nil {
			failures = append(failures, err.Error())
			return
		}
		parsed := int(value.(lua.LNumber))
		if parsed < 0 || parsed > fontdesc.FeatureValueMaximum {
			failures = append(failures, fmt.Sprintf("%s: must be between 0 and %d", entryPath, fontdesc.FeatureValueMaximum))
			return
		}
		concrete++
	})
	if concrete > fontdesc.MaxFeatureTags {
		failures = append(failures, fmt.Sprintf("%s: must contain at most %d concrete entries", path, fontdesc.MaxFeatureTags))
	}
	if len(failures) != 0 {
		sort.Strings(failures)
		return fmt.Errorf("%s", failures[0])
	}
	return nil
}

func validateStringMap(source, path string, value lua.LValue, allowUnset bool) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindStringMap, value)
	}
	var failures []string
	table.ForEach(func(key, value lua.LValue) {
		stringKey, keyOK := key.(lua.LString)
		_, stringValue := value.(lua.LString)
		valueOK := stringValue || (allowUnset && isUnsetValue(value))
		if !keyOK {
			failures = append(failures, fmt.Sprintf("%s: map key must be string, got %s", path, key.Type().String()))
		} else if !valueOK {
			expected := "string"
			if allowUnset {
				expected = "string or cervterm.config.unset"
			}
			failures = append(failures, fmt.Sprintf("%s.%s: must be %s, got %s", path, string(stringKey), expected, value.Type().String()))
		}
	})
	if len(failures) > 0 {
		sort.Strings(failures)
		return fmt.Errorf("%s: %s", sourceLabel(source), failures[0])
	}
	return nil
}

func validateDocumentMap(source, path string, value lua.LValue) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindDocumentMap, value)
	}
	names, err := strictStringKeys(source, path, table)
	if err != nil {
		return err
	}
	for _, name := range names {
		if name == "" {
			return documentError(source, path, "name must not be empty")
		}
		partial, ok := table.RawGetString(name).(*lua.LTable)
		if !ok {
			return typeError(source, joinPath(path, name), KindTable, table.RawGetString(name))
		}
		if err := validateStrictTable(source, joinPath(path, name), partial, rootSchema, false, nil, true); err != nil {
			return err
		}
	}
	return nil
}

func validateColorSchemeMap(source, path string, value lua.LValue, _ bool) error {
	table, ok := value.(*lua.LTable)
	if !ok {
		return typeError(source, path, KindColorSchemeMap, value)
	}
	names, err := strictStringKeys(source, path, table)
	if err != nil {
		return err
	}
	for _, name := range names {
		if name == "" {
			return documentError(source, path, "name must not be empty")
		}
		palette, ok := table.RawGetString(name).(*lua.LTable)
		if !ok {
			return typeError(source, mapEntryPath(path, name), KindTable, table.RawGetString(name))
		}
		palettePath := mapEntryPath(path, name)
		if err := validateStrictTable(source, palettePath, palette, colorSchemeSchema, false, nil, true); err != nil {
			return err
		}
		for _, field := range []string{"foreground", "background", "cursor", "selection_background", "chrome_background", "chrome_muted", "accent", "split", "search_match", "error"} {
			entry := palette.RawGetString(field)
			if entry == lua.LNil || isUnsetValue(entry) {
				continue
			}
			if !isHexColor(string(entry.(lua.LString))) {
				return documentError(source, joinPath(palettePath, field), "must be #RRGGBB or #RRGGBBAA")
			}
		}
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

func validateEvents(source, path string, value lua.LValue, allowUnset bool) error {
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
		if value := table.RawGetString(key); value != lua.LNil && value.Type() != lua.LTFunction && !(allowUnset && isUnsetValue(value)) {
			expected := "a function"
			if allowUnset {
				expected = "a function or cervterm.config.unset"
			}
			return documentError(source, path+"."+key, "must be %s", expected)
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
