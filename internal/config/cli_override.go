package config

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

type CLIOverride struct {
	ArgumentIndex int
	Path          string
	Value         string
}

type resolvedOverridePath struct {
	field     fieldSchema
	parts     []string
	sensitive bool
}

type cliFontDescriptor struct {
	Family          json.RawMessage `json:"family"`
	CollectionFace  json.RawMessage `json:"collection_face"`
	CollectionIndex json.RawMessage `json:"collection_index"`
	Weight          json.RawMessage `json:"weight"`
	Style           json.RawMessage `json:"style"`
	Stretch         json.RawMessage `json:"stretch"`
	AttributeMode   json.RawMessage `json:"attribute_mode"`
}

func cliOverrideKindAllowed(kind ValueKind) bool {
	switch kind {
	case KindString, KindNumber, KindInteger, KindBoolean, KindStringList, KindFeatureMap, KindDescriptorList, KindFontRuleList:
		return true
	default:
		return false
	}
}

func resolveCLIOverridePath(path string) (resolvedOverridePath, error) {
	if path == "" || strings.TrimSpace(path) != path {
		return resolvedOverridePath{}, fmt.Errorf("path must be a non-empty canonical dotted field name")
	}
	parts := strings.Split(path, ".")
	current := rootSchema
	sensitive := false
	for index, part := range parts {
		var child *fieldSchema
		for i := range current.children {
			if current.children[i].name == part {
				child = &current.children[i]
				break
			}
		}
		if child == nil {
			return resolvedOverridePath{}, fmt.Errorf("unknown configuration path %q", path)
		}
		sensitive = sensitive || child.sensitive
		if index == len(parts)-1 {
			return resolvedOverridePath{field: *child, parts: parts, sensitive: sensitive}, nil
		}
		if child.kind != KindTable {
			if sensitive {
				return resolvedOverridePath{field: *child, parts: parts[:index+1], sensitive: true}, nil
			}
			return resolvedOverridePath{}, fmt.Errorf("configuration path %q does not name a schema field", path)
		}
		current = *child
	}
	return resolvedOverridePath{}, fmt.Errorf("unknown configuration path %q", path)
}

func decodeCLIOverrideValue(state *lua.LState, path resolvedOverridePath, raw string) (lua.LValue, int, error) {
	switch path.field.kind {
	case KindString:
		if strings.HasPrefix(raw, `"`) {
			var value string
			if err := json.Unmarshal([]byte(raw), &value); err != nil {
				return nil, 0, fmt.Errorf("must be a JSON string or an unquoted string")
			}
			return lua.LString(value), 1, nil
		}
		return lua.LString(raw), 1, nil
	case KindBoolean:
		if raw == "null" {
			return nil, 0, fmt.Errorf("must be a JSON boolean")
		}
		var value bool
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, 0, fmt.Errorf("must be a JSON boolean")
		}
		return lua.LBool(value), 1, nil
	case KindNumber:
		if raw == "null" {
			return nil, 0, fmt.Errorf("must be a finite JSON number")
		}
		var value float64
		if err := json.Unmarshal([]byte(raw), &value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, 0, fmt.Errorf("must be a finite JSON number")
		}
		return lua.LNumber(value), 1, nil
	case KindInteger:
		value, err := exactCLIInteger(raw)
		if err != nil {
			return nil, 0, err
		}
		return lua.LNumber(value), 1, nil
	case KindStringList:
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err != nil || values == nil {
			return nil, 0, fmt.Errorf("must be a JSON array of strings")
		}
		table := state.NewTable()
		for _, value := range values {
			table.Append(lua.LString(value))
		}
		return table, len(values) + 1, nil
	case KindDescriptorList:
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		decoder.DisallowUnknownFields()
		var values []cliFontDescriptor
		if err := decoder.Decode(&values); err != nil || values == nil {
			return nil, 0, fmt.Errorf("must be a JSON array of font descriptor objects")
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return nil, 0, fmt.Errorf("must contain exactly one JSON array")
		}
		table := state.NewTable()
		for _, value := range values {
			entry := state.NewTable()
			for _, field := range []struct {
				name string
				raw  json.RawMessage
			}{
				{name: "family", raw: value.Family},
				{name: "collection_face", raw: value.CollectionFace},
				{name: "style", raw: value.Style},
				{name: "attribute_mode", raw: value.AttributeMode},
			} {
				if len(field.raw) == 0 {
					continue
				}
				var text string
				if string(field.raw) == "null" || json.Unmarshal(field.raw, &text) != nil {
					return nil, 0, fmt.Errorf("descriptor %s must be a JSON string", field.name)
				}
				entry.RawSetString(field.name, lua.LString(text))
			}
			for _, field := range []struct {
				name string
				raw  json.RawMessage
			}{
				{name: "collection_index", raw: value.CollectionIndex},
				{name: "weight", raw: value.Weight},
				{name: "stretch", raw: value.Stretch},
			} {
				if len(field.raw) == 0 {
					continue
				}
				parsed, err := exactCLIInteger(string(field.raw))
				if err != nil {
					return nil, 0, fmt.Errorf("descriptor %s: %w", field.name, err)
				}
				entry.RawSetString(field.name, lua.LNumber(parsed))
			}
			table.Append(entry)
		}
		return table, 1 + len(values)*8, nil
	case KindFeatureMap:
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var values map[string]any
		if err := decoder.Decode(&values); err != nil || values == nil {
			return nil, 0, fmt.Errorf("must be a JSON object of feature integers or null tombstones")
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return nil, 0, fmt.Errorf("must contain exactly one JSON object")
		}
		table := state.NewTable()
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if values[key] == nil {
				table.RawSetString(key, NewUnsetValue(state))
				continue
			}
			number, ok := values[key].(json.Number)
			if !ok {
				return nil, 0, fmt.Errorf("feature %q must be a JSON integer or null", key)
			}
			parsed, err := exactCLIInteger(number.String())
			if err != nil {
				return nil, 0, fmt.Errorf("feature %q: %w", key, err)
			}
			table.RawSetString(key, lua.LNumber(parsed))
		}
		return table, len(values) + 1, nil
	case KindFontRuleList:
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, 0, fmt.Errorf("must be a JSON array of font rule objects")
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return nil, 0, fmt.Errorf("must contain exactly one JSON array")
		}
		values, ok := value.([]any)
		if !ok || values == nil {
			return nil, 0, fmt.Errorf("must be a JSON array of font rule objects")
		}
		converted, cost, err := cliJSONValueToLua(state, values)
		if err != nil {
			return nil, 0, err
		}
		return converted, cost, nil
	default:
		return nil, 0, fmt.Errorf("field kind %s is not CLI-overridable", path.field.kind)
	}
}

func exactCLIInteger(raw string) (float64, error) {
	if !json.Valid([]byte(raw)) || raw == "null" {
		return 0, fmt.Errorf("must be a JSON integer")
	}
	rational, ok := new(big.Rat).SetString(raw)
	if !ok || !rational.IsInt() {
		return 0, fmt.Errorf("must be a JSON integer")
	}
	upper := new(big.Int).Lsh(big.NewInt(1), uint(strconv.IntSize-1))
	lower := new(big.Int).Neg(new(big.Int).Set(upper))
	integer := rational.Num()
	if integer.Cmp(lower) < 0 || integer.Cmp(upper) >= 0 {
		return 0, fmt.Errorf("must be an integer in [%s, %s)", lower.String(), upper.String())
	}
	value, _ := new(big.Float).SetInt(integer).Float64()
	roundTrip := new(big.Rat).SetFloat64(value)
	if roundTrip == nil || roundTrip.Cmp(rational) != 0 {
		return 0, fmt.Errorf("must be exactly representable as a terminal configuration integer")
	}
	return value, nil
}

func cliJSONValueToLua(state *lua.LState, value any) (lua.LValue, int, error) {
	switch typed := value.(type) {
	case string:
		return lua.LString(typed), 1, nil
	case bool:
		return lua.LBool(typed), 1, nil
	case json.Number:
		parsed, err := exactCLIInteger(typed.String())
		if err != nil {
			return nil, 0, err
		}
		return lua.LNumber(parsed), 1, nil
	case []any:
		table := state.NewTable()
		cost := 1
		for _, entry := range typed {
			converted, entryCost, err := cliJSONValueToLua(state, entry)
			if err != nil {
				return nil, 0, err
			}
			table.Append(converted)
			cost += entryCost
		}
		return table, cost, nil
	case map[string]any:
		table := state.NewTable()
		cost := 1
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			converted, entryCost, err := cliJSONValueToLua(state, typed[key])
			if err != nil {
				return nil, 0, fmt.Errorf("%s: %w", key, err)
			}
			table.RawSetString(key, converted)
			cost += 1 + entryCost
		}
		return table, cost, nil
	case nil:
		return nil, 0, fmt.Errorf("JSON null is not valid in font rules")
	default:
		return nil, 0, fmt.Errorf("unsupported JSON value %T", value)
	}
}

func (b *compositionBuilder) applyCLIOverrides(overrides []CLIOverride) error {
	for _, override := range overrides {
		if override.ArgumentIndex < 0 {
			return fmt.Errorf("config override path %q has negative argument index", override.Path)
		}
		resolved, err := resolveCLIOverridePath(override.Path)
		if err != nil {
			return fmt.Errorf("config override argument %d path %q: %w", override.ArgumentIndex, override.Path, err)
		}
		if resolved.sensitive {
			return fmt.Errorf("config override argument %d path %q: sensitive fields cannot be supplied through process arguments", override.ArgumentIndex, override.Path)
		}
		if !cliOverrideKindAllowed(resolved.field.kind) {
			return fmt.Errorf("config override argument %d path %q: field kind %s is not CLI-overridable", override.ArgumentIndex, override.Path, resolved.field.kind)
		}
		value, cost, err := decodeCLIOverrideValue(b.state, resolved, override.Value)
		if err != nil {
			return fmt.Errorf("config override argument %d path %q: %w", override.ArgumentIndex, override.Path, err)
		}
		if err := validateStrictValue("CLI override", override.Path, value, resolved.field, resolved.field.kind == KindFeatureMap); err != nil {
			return fmt.Errorf("config override argument %d path %q: %w", override.ArgumentIndex, override.Path, err)
		}
		if err := b.consume(cost, override.Path); err != nil {
			return err
		}
		target := b.root
		for _, part := range resolved.parts[:len(resolved.parts)-1] {
			nested, ok := target.RawGetString(part).(*lua.LTable)
			if !ok {
				nested = b.state.NewTable()
				target.RawSetString(part, nested)
			}
			target = nested
		}
		origin := ProvenanceOrigin{
			Layer: LayerCLI, Name: "--config-override",
			CLIArgumentIndex: override.ArgumentIndex, HasCLIArgumentIndex: true,
		}
		if resolved.field.kind == KindFeatureMap {
			table := value.(*lua.LTable)
			existing, ok := target.RawGetString(resolved.parts[len(resolved.parts)-1]).(*lua.LTable)
			if !ok {
				existing = b.state.NewTable()
				target.RawSetString(resolved.parts[len(resolved.parts)-1], existing)
			}
			keys := make([]string, 0, table.Len())
			table.ForEach(func(key, _ lua.LValue) {
				if text, ok := key.(lua.LString); ok {
					keys = append(keys, string(text))
				}
			})
			sort.Strings(keys)
			for _, key := range keys {
				entry := table.RawGetString(key)
				tombstone := isUnsetValue(entry)
				if tombstone {
					existing.RawSetString(key, lua.LNil)
				} else {
					existing.RawSetString(key, entry)
				}
				b.provenance.set(mapEntryPath(override.Path, key), origin, tombstone, false)
			}
			continue
		}
		target.RawSetString(resolved.parts[len(resolved.parts)-1], value)
		b.provenance.set(override.Path, origin, false, false)
	}
	return nil
}
