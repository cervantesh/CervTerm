package config

import (
	"fmt"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

var colorSchemeSchema = fieldSchema{kind: KindTable, children: []fieldSchema{
	{name: "foreground", kind: KindString},
	{name: "background", kind: KindString},
	{name: "cursor", kind: KindString},
	{name: "selection_background", kind: KindString},
	{name: "chrome_background", kind: KindString},
	{name: "chrome_muted", kind: KindString},
	{name: "accent", kind: KindString},
	{name: "split", kind: KindString},
	{name: "search_match", kind: KindString},
	{name: "error", kind: KindString},
	{name: "ansi", kind: KindStringList},
	{name: "indexed_colors", kind: KindIndexedColorMap},
}}

var colorsSchema = func() fieldSchema {
	for _, field := range rootSchema.children {
		if field.name == "colors" {
			return field
		}
	}
	panic("config colors schema is missing")
}()

type namedColorScheme struct {
	palette    *lua.LTable
	provenance map[string]ProvenanceRecord
}

type namedColorSchemeCatalog map[string]*namedColorScheme

func (b *compositionBuilder) mergeColorSchemeDeclarations(graph *SourceGraph, source SourceNode, catalog namedColorSchemeCatalog) error {
	declarations, ok := source.Document.Root.RawGetString("color_schemes").(*lua.LTable)
	if !ok {
		return nil
	}
	if err := b.consume(1, "color_schemes"); err != nil {
		return err
	}
	names, _ := strictStringKeys(source.Document.Source, "color_schemes", declarations)
	for _, name := range names {
		if err := b.consume(1, mapEntryPath("color_schemes", name)); err != nil {
			return err
		}
		palette := declarations.RawGetString(name).(*lua.LTable)
		scheme := catalog[name]
		if scheme == nil {
			scheme = &namedColorScheme{palette: b.state.NewTable(), provenance: make(map[string]ProvenanceRecord)}
			catalog[name] = scheme
		}
		origin := sourceLayerOrigin(graph, source, LayerColorScheme, name)
		if err := b.mergeColorSchemePalette(scheme, palette, origin, mapEntryPath("color_schemes", name)); err != nil {
			return err
		}
	}
	return nil
}

func (b *compositionBuilder) mergeColorSchemePalette(scheme *namedColorScheme, source *lua.LTable, origin ProvenanceOrigin, path string) error {
	for _, field := range colorSchemeSchema.children {
		value := source.RawGetString(field.name)
		if value == lua.LNil {
			continue
		}
		fieldPath := joinPath(path, field.name)
		if isUnsetValue(value) {
			cost := 1
			if field.kind == KindIndexedColorMap {
				cost = 0
				for key := range scheme.provenance {
					if strings.HasPrefix(key, "indexed_colors[") {
						cost++
					}
				}
				if cost == 0 {
					cost = 1
				}
			}
			if err := b.consume(cost, fieldPath); err != nil {
				return err
			}
			scheme.palette.RawSetString(field.name, lua.LNil)
			if field.kind == KindIndexedColorMap {
				for key := range scheme.provenance {
					if strings.HasPrefix(key, "indexed_colors[") {
						scheme.setProvenance(key, origin, true)
					}
				}
			} else {
				scheme.setProvenance(field.name, origin, true)
			}
			continue
		}
		if field.kind != KindIndexedColorMap {
			cost := 1
			if table, ok := value.(*lua.LTable); ok {
				cost += table.Len()
			}
			if err := b.consume(cost, fieldPath); err != nil {
				return err
			}
			scheme.palette.RawSetString(field.name, value)
			scheme.setProvenance(field.name, origin, false)
			continue
		}
		sourceIndexed := value.(*lua.LTable)
		target, ok := scheme.palette.RawGetString(field.name).(*lua.LTable)
		if !ok {
			target = b.state.NewTable()
			scheme.palette.RawSetString(field.name, target)
		}
		for _, key := range indexedColorKeys(sourceIndexed) {
			entryPath := indexedColorEntryPath(fieldPath, key)
			if err := b.consume(1, entryPath); err != nil {
				return err
			}
			provenanceKey := indexedColorEntryPath("indexed_colors", key)
			entry := sourceIndexed.RawGetInt(key)
			if isUnsetValue(entry) {
				target.RawSetInt(key, lua.LNil)
				scheme.setProvenance(provenanceKey, origin, true)
				continue
			}
			target.RawSetInt(key, entry)
			scheme.setProvenance(provenanceKey, origin, false)
		}
	}
	return nil
}

func (s *namedColorScheme) setProvenance(path string, origin ProvenanceOrigin, tombstone bool) {
	record := ProvenanceRecord{Path: path, Winner: origin, Tombstone: tombstone}
	if previous, ok := s.provenance[path]; ok {
		record.Overwritten = append(record.Overwritten, previous.Overwritten...)
		record.Overwritten = append(record.Overwritten, previous.Winner)
	}
	s.provenance[path] = record
}

func (b *compositionBuilder) applySelectedColorScheme(catalog namedColorSchemeCatalog) error {
	value := b.root.RawGetString("color_scheme")
	selector, ok := value.(lua.LString)
	if !ok {
		return nil
	}
	name := string(selector)
	scheme := catalog[name]
	if scheme == nil {
		if record, found := b.provenance.Lookup("color_scheme"); found {
			return fmt.Errorf("selected color scheme %q is not declared (selected by %s %q from %q)", name, record.Winner.Layer, record.Winner.Name, record.Winner.CanonicalSource)
		}
		return fmt.Errorf("selected color scheme %q is not declared", name)
	}
	colors, ok := b.root.RawGetString("colors").(*lua.LTable)
	if !ok {
		colors = b.state.NewTable()
		b.root.RawSetString("colors", colors)
	}
	for _, field := range colorSchemeSchema.children {
		value := scheme.palette.RawGetString(field.name)
		path := joinPath("colors", field.name)
		if field.kind != KindIndexedColorMap {
			if record, exists := scheme.provenance[field.name]; exists {
				b.applySchemeProvenance(path, record)
			}
			if value != lua.LNil {
				colors.RawSetString(field.name, cloneLuaValue(b.state, value))
			}
			continue
		}
		indexed := b.state.NewTable()
		if value != lua.LNil {
			colors.RawSetString(field.name, indexed)
			sourceIndexed := value.(*lua.LTable)
			for _, key := range indexedColorKeys(sourceIndexed) {
				indexed.RawSetInt(key, sourceIndexed.RawGetInt(key))
			}
		}
		for key, record := range scheme.provenance {
			if strings.HasPrefix(key, "indexed_colors[") {
				b.applySchemeProvenance(strings.Replace(key, "indexed_colors", "colors.indexed_colors", 1), record)
			}
		}
	}
	return nil
}

func (b *compositionBuilder) applySchemeProvenance(path string, record ProvenanceRecord) {
	for _, origin := range record.Overwritten {
		b.provenance.set(path, origin, false, false)
	}
	b.provenance.set(path, record.Winner, record.Tombstone, false)
}

func (b *compositionBuilder) mergeExplicitColors(root *lua.LTable) error {
	value := root.RawGetString("colors")
	if value == lua.LNil {
		return nil
	}
	if isUnsetValue(value) {
		if err := b.consume(b.unsetCost(colorsSchema, "colors"), "colors"); err != nil {
			return err
		}
		b.unset(b.root, colorsSchema, "colors")
		return nil
	}
	source, ok := value.(*lua.LTable)
	if !ok {
		return nil
	}
	target, ok := b.root.RawGetString("colors").(*lua.LTable)
	if !ok {
		target = b.state.NewTable()
		b.root.RawSetString("colors", target)
	}
	for _, field := range colorsSchema.children {
		entry := source.RawGetString(field.name)
		if entry == lua.LNil {
			continue
		}
		if b.document.AuthoredVersion == 1 && !legacyValueCompatible(entry, field.kind) {
			continue
		}
		path := joinPath("colors", field.name)
		if isUnsetValue(entry) {
			if err := b.consume(b.unsetCost(field, path), path); err != nil {
				return err
			}
			b.unset(target, field, path)
			continue
		}
		if field.kind == KindIndexedColorMap {
			if err := b.mergeExplicitIndexedColors(target, field, path, entry.(*lua.LTable)); err != nil {
				return err
			}
			continue
		}
		cost := 1
		if table, ok := entry.(*lua.LTable); ok {
			cost += table.Len()
		}
		if err := b.consume(cost, path); err != nil {
			return err
		}
		target.RawSetString(field.name, entry)
		b.provenance.set(path, b.origin, false, false)
	}
	return nil
}

func (b *compositionBuilder) mergeExplicitIndexedColors(target *lua.LTable, field fieldSchema, path string, source *lua.LTable) error {
	indexed, ok := target.RawGetString(field.name).(*lua.LTable)
	if !ok {
		indexed = b.state.NewTable()
		target.RawSetString(field.name, indexed)
	}
	for _, key := range indexedColorKeys(source) {
		entryPath := indexedColorEntryPath(path, key)
		if err := b.consume(1, entryPath); err != nil {
			return err
		}
		entry := source.RawGetInt(key)
		if isUnsetValue(entry) {
			indexed.RawSetInt(key, lua.LNil)
			b.provenance.set(entryPath, b.origin, true, false)
			continue
		}
		indexed.RawSetInt(key, entry)
		b.provenance.set(entryPath, b.origin, false, false)
	}
	return nil
}

func cloneLuaValue(state *lua.LState, value lua.LValue) lua.LValue {
	table, ok := value.(*lua.LTable)
	if !ok {
		return value
	}
	clone := state.NewTable()
	table.ForEach(func(key, entry lua.LValue) {
		clone.RawSet(key, cloneLuaValue(state, entry))
	})
	return clone
}

func colorOverrides(overrides []CLIOverride, wantColors bool) []CLIOverride {
	filtered := make([]CLIOverride, 0, len(overrides))
	for _, override := range overrides {
		isColor := override.Path == "colors" || strings.HasPrefix(override.Path, "colors.")
		if isColor == wantColors {
			filtered = append(filtered, override)
		}
	}
	return filtered
}

func sortedSchemeNames(catalog namedColorSchemeCatalog) []string {
	names := make([]string, 0, len(catalog))
	for name := range catalog {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
