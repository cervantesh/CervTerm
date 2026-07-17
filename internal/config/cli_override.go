package config

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
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

func cliOverrideKindAllowed(kind ValueKind) bool {
	switch kind {
	case KindString, KindNumber, KindInteger, KindBoolean, KindStringList:
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
		if !json.Valid([]byte(raw)) || raw == "null" {
			return nil, 0, fmt.Errorf("must be a JSON integer")
		}
		var number json.Number
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&number); err != nil {
			return nil, 0, fmt.Errorf("must be a JSON integer")
		}
		rational, ok := new(big.Rat).SetString(number.String())
		if !ok || !rational.IsInt() {
			return nil, 0, fmt.Errorf("must be a JSON integer")
		}
		upper := new(big.Int).Lsh(big.NewInt(1), uint(strconv.IntSize-1))
		lower := new(big.Int).Neg(new(big.Int).Set(upper))
		integer := rational.Num()
		if integer.Cmp(lower) < 0 || integer.Cmp(upper) >= 0 {
			return nil, 0, fmt.Errorf("must be an integer in [%s, %s)", lower.String(), upper.String())
		}
		value, _ := new(big.Float).SetInt(integer).Float64()
		roundTrip := new(big.Rat).SetFloat64(value)
		if roundTrip == nil || roundTrip.Cmp(rational) != 0 {
			return nil, 0, fmt.Errorf("must be exactly representable as a terminal configuration integer")
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
	default:
		return nil, 0, fmt.Errorf("field kind %s is not CLI-overridable", path.field.kind)
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
		if err := validateStrictValue("CLI override", override.Path, value, resolved.field, false); err != nil {
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
		target.RawSetString(resolved.parts[len(resolved.parts)-1], value)
		origin := ProvenanceOrigin{
			Layer: LayerCLI, Name: "--config-override",
			CLIArgumentIndex: override.ArgumentIndex, HasCLIArgumentIndex: true,
		}
		b.provenance.set(override.Path, origin, false, false)
	}
	return nil
}
