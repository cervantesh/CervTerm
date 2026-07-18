package config

import (
	"fmt"
	"math"
	"sort"

	lua "github.com/yuin/gopher-lua"
)

const (
	MinimumSchemaVersion = 1
	CurrentSchemaVersion = 2
)

type ValueKind string

const (
	KindTable           ValueKind = "table"
	KindString          ValueKind = "string"
	KindNumber          ValueKind = "number"
	KindInteger         ValueKind = "integer"
	KindBoolean         ValueKind = "boolean"
	KindStringList      ValueKind = "string_list"
	KindStringMap       ValueKind = "string_map"
	KindFeatureMap      ValueKind = "feature_map"
	KindIndexedColorMap ValueKind = "indexed_color_map"
	KindKeyList         ValueKind = "key_list"
	KindEvents          ValueKind = "events"
	KindDocumentMap     ValueKind = "document_map"
	KindDescriptorList  ValueKind = "descriptor_list"
	KindFontRuleList    ValueKind = "font_rule_list"
	KindColorSchemeMap  ValueKind = "color_scheme_map"
)

type ApplyScope string

const (
	ApplyLive           ApplyScope = "live"
	ApplyNewPane        ApplyScope = "new_pane"
	ApplyNewWindow      ApplyScope = "new_window"
	ApplyWindowRecreate ApplyScope = "window_recreate"
	ApplyRestart        ApplyScope = "restart"
)

type FieldMetadata struct {
	Path            string
	Kind            ValueKind
	Available       bool
	Sensitive       bool
	CLIOverride     bool
	ApplyScope      ApplyScope
	RuntimeOverride bool
}

type MigrationStep struct {
	From int
	To   int
}

type Document struct {
	Source          string
	AuthoredVersion int
	Version         int
	Root            *lua.LTable
	Present         map[string]struct{}
	Migrations      []MigrationStep
}

func (d Document) Has(path string) bool {
	_, ok := d.Present[path]
	return ok
}

func FromDocument(base Config, document Document) Config {
	cfg := FromTable(base, document.Root)
	if document.AuthoredVersion >= 2 {
		cfg.ColorScheme = stringField(document.Root, "color_scheme", cfg.ColorScheme)
		if font := tableField(document.Root, "font"); font != nil {
			cfg.Font.Descriptors = descriptorListField(font, "descriptors", cfg.Font.Descriptors)
			cfg.Font.Fallback = descriptorListField(font, "fallback", cfg.Font.Fallback)
			cfg.Font.Rules = fontRuleListField(font, "rules", cfg.Font.Rules)
			cfg.Font.Features = integerMapField(font, "features", cfg.Font.Features)
		}
	}
	return cfg
}

type fieldSchema struct {
	name            string
	kind            ValueKind
	required        bool
	children        []fieldSchema
	sensitive       bool
	apply           ApplyScope
	runtimeOverride bool
}

var rootSchema = fieldSchema{kind: KindTable, children: []fieldSchema{
	{name: "window", kind: KindTable, children: []fieldSchema{
		{name: "width", kind: KindInteger, apply: ApplyNewWindow}, {name: "height", kind: KindInteger, apply: ApplyNewWindow},
		{name: "padding_x", kind: KindInteger, apply: ApplyRestart}, {name: "padding_y", kind: KindInteger, apply: ApplyRestart},
		{name: "dynamic_title", kind: KindBoolean, apply: ApplyRestart},
		{name: "opacity", kind: KindNumber, apply: ApplyLive, runtimeOverride: true},
		{name: "blur", kind: KindBoolean, apply: ApplyLive, runtimeOverride: true},
	}},
	{name: "font", kind: KindTable, apply: ApplyRestart, children: []fieldSchema{
		{name: "family", kind: KindString}, {name: "descriptors", kind: KindDescriptorList}, {name: "fallback", kind: KindDescriptorList}, {name: "rules", kind: KindFontRuleList},
		{name: "size", kind: KindNumber}, {name: "ligatures", kind: KindBoolean}, {name: "features", kind: KindFeatureMap},
	}},
	{name: "color_scheme", kind: KindString, apply: ApplyLive},
	{name: "colors", kind: KindTable, apply: ApplyLive, children: []fieldSchema{
		{name: "foreground", kind: KindString}, {name: "background", kind: KindString, runtimeOverride: true},
		{name: "cursor", kind: KindString}, {name: "selection_background", kind: KindString},
		{name: "chrome_background", kind: KindString}, {name: "chrome_muted", kind: KindString},
		{name: "accent", kind: KindString}, {name: "split", kind: KindString},
		{name: "search_match", kind: KindString}, {name: "error", kind: KindString},
		{name: "ansi", kind: KindStringList}, {name: "indexed_colors", kind: KindIndexedColorMap},
	}},
	{name: "scrolling", kind: KindTable, apply: ApplyLive, runtimeOverride: true, children: []fieldSchema{
		{name: "history", kind: KindInteger}, {name: "wheel_multiplier", kind: KindInteger}, {name: "hide_cursor_when_scrolled", kind: KindBoolean},
	}},
	{name: "scrollbar", kind: KindTable, apply: ApplyLive, runtimeOverride: true, children: []fieldSchema{
		{name: "enabled", kind: KindBoolean}, {name: "reserved_width_px", kind: KindInteger},
		{name: "width_px", kind: KindInteger}, {name: "margin_px", kind: KindInteger},
		{name: "radius_px", kind: KindInteger}, {name: "min_thumb_px", kind: KindInteger},
		{name: "track_color", kind: KindString}, {name: "thumb_color", kind: KindString},
		{name: "thumb_hover_color", kind: KindString}, {name: "thumb_press_color", kind: KindString},
		{name: "auto_hide_delay_ms", kind: KindInteger}, {name: "fade_ms", kind: KindInteger},
		{name: "page_step", kind: KindNumber}, {name: "track_click", kind: KindString},
	}},
	{name: "cursor", kind: KindTable, apply: ApplyLive, children: []fieldSchema{
		{name: "shape", kind: KindString}, {name: "blink", kind: KindBoolean},
		{name: "blink_interval_ms", kind: KindInteger}, {name: "thickness", kind: KindNumber},
	}},
	{name: "clipboard", kind: KindTable, apply: ApplyRestart, children: []fieldSchema{{name: "osc52", kind: KindString}}},
	{name: "render", kind: KindTable, apply: ApplyRestart, children: []fieldSchema{
		{name: "bidi", kind: KindBoolean}, {name: "text_gamma", kind: KindNumber}, {name: "text_darken", kind: KindNumber},
		{name: "text_raster", kind: KindString}, {name: "stats_hotkey", kind: KindString},
		{name: "zoom_in_hotkey", kind: KindString}, {name: "zoom_out_hotkey", kind: KindString},
		{name: "zoom_reset_hotkey", kind: KindString}, {name: "vsync", kind: KindBoolean},
		{name: "redraw", kind: KindString}, {name: "damage", kind: KindString},
	}},
	{name: "shell", kind: KindTable, apply: ApplyNewPane, children: []fieldSchema{
		{name: "program", kind: KindString}, {name: "args", kind: KindStringList},
		{name: "working_directory", kind: KindString}, {name: "env", kind: KindStringMap, sensitive: true},
	}},
	{name: "keys", kind: KindKeyList, apply: ApplyLive},
	{name: "events", kind: KindEvents, apply: ApplyLive},
}}

var unavailableV2Fields = map[string]ValueKind{
	"includes": KindStringList, "default_environment": KindString, "default_profile": KindString,
	"environments": KindDocumentMap, "profiles": KindDocumentMap, "color_schemes": KindColorSchemeMap,
}

func SchemaFields(version int) ([]FieldMetadata, error) {
	if version < MinimumSchemaVersion || version > CurrentSchemaVersion {
		return nil, fmt.Errorf("unsupported config schema version %d", version)
	}
	fields := []FieldMetadata{{Path: "config_version", Kind: KindInteger, Available: true}}
	var walk func(prefix string, schema fieldSchema, inheritedApply ApplyScope, inheritedRuntime bool)
	walk = func(prefix string, schema fieldSchema, inheritedApply ApplyScope, inheritedRuntime bool) {
		for _, child := range schema.children {
			if version == 1 && prefix == "" && child.name == "color_scheme" {
				continue
			}
			path := child.name
			if prefix != "" {
				path = prefix + "." + child.name
			}
			if version == 1 && (path == "font.descriptors" || path == "font.fallback" || path == "font.rules" || path == "font.features") {
				continue
			}
			apply := child.apply
			if apply == "" {
				apply = inheritedApply
			}
			runtimeOverride := child.runtimeOverride || inheritedRuntime
			metadata := FieldMetadata{Path: path, Kind: child.kind, Available: true, Sensitive: child.sensitive, CLIOverride: cliOverrideKindAllowed(child.kind) && !child.sensitive}
			if child.kind != KindTable {
				metadata.ApplyScope = apply
				metadata.RuntimeOverride = runtimeOverride
			}
			fields = append(fields, metadata)
			if child.kind == KindTable {
				walk(path, child, apply, runtimeOverride)
			}
		}
	}
	walk("", rootSchema, "", false)
	if version == 2 {
		for _, name := range []string{"includes", "default_environment", "default_profile", "environments", "profiles", "color_schemes"} {
			fields = append(fields, FieldMetadata{Path: name, Kind: unavailableV2Fields[name], Available: false})
		}
	}
	return fields, nil
}

func DecodeDocument(source string, root *lua.LTable) (Document, error) {
	return decodeDocument(source, root, nil)
}

func decodeDocument(source string, root *lua.LTable, available map[string]fieldSchema) (Document, error) {
	return decodeDocumentOptions(source, root, available, false)
}

func decodeCompositionDocument(source string, root *lua.LTable, available map[string]fieldSchema) (Document, error) {
	return decodeDocumentOptions(source, root, available, true)
}

func decodeDocumentOptions(source string, root *lua.LTable, available map[string]fieldSchema, allowUnset bool) (Document, error) {
	version, err := documentVersion(source, root)
	if err != nil {
		return Document{}, err
	}
	if unsetPath := findUnsetPath(root, rootSchema, ""); unsetPath != "" {
		if version == 1 {
			return Document{}, documentError(source, unsetPath, "cervterm.config.unset requires config_version = 2")
		}
		if !allowUnset {
			return Document{}, documentError(source, unsetPath, "cervterm.config.unset is not available in single-source loading")
		}
	}
	document := Document{
		Source: source, AuthoredVersion: version, Version: version,
		Root: root, Present: make(map[string]struct{}),
	}
	collectPresence(root, rootSchema, "", document.Present)
	if version == 1 {
		delete(document.Present, "color_scheme")
		delete(document.Present, "font.descriptors")
		delete(document.Present, "font.fallback")
		delete(document.Present, "font.rules")
		delete(document.Present, "font.features")
	}
	if root.RawGetString("config_version") != lua.LNil {
		document.Present["config_version"] = struct{}{}
	}
	for name := range available {
		if root.RawGetString(name) != lua.LNil {
			document.Present[name] = struct{}{}
		}
	}
	if err := rejectUnavailableComposition(source, root, version, available); err != nil {
		return Document{}, err
	}
	if version == 2 {
		if err := validateStrictTable(source, "", root, rootSchema, true, available, allowUnset); err != nil {
			return Document{}, err
		}
	}
	for document.Version < CurrentSchemaVersion {
		from := document.Version
		if err := migrateDocument(&document, from); err != nil {
			return Document{}, fmt.Errorf("%s: migrate config v%d to v%d: %w", sourceLabel(source), from, from+1, err)
		}
		document.Version++
		document.Migrations = append(document.Migrations, MigrationStep{From: from, To: document.Version})
	}
	return document, nil
}

func documentVersion(source string, root *lua.LTable) (int, error) {
	value := root.RawGetString("config_version")
	if value == lua.LNil {
		return 1, nil
	}
	number, ok := value.(lua.LNumber)
	if !ok {
		return 0, documentError(source, "config_version", "must be an integer, got %s", value.Type().String())
	}
	parsed := float64(number)
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) || math.Trunc(parsed) != parsed {
		return 0, documentError(source, "config_version", "must be a finite integer")
	}
	if parsed < MinimumSchemaVersion {
		return 0, documentError(source, "config_version", "version %g is older than the minimum supported version %d", parsed, MinimumSchemaVersion)
	}
	if parsed > CurrentSchemaVersion {
		return 0, documentError(source, "config_version", "version %g requires a newer CervTerm (current version is %d)", parsed, CurrentSchemaVersion)
	}
	return int(parsed), nil
}

func migrateDocument(document *Document, from int) error {
	switch from {
	case 1:
		// V1 and V2 currently share ordinary field names. This explicit no-op step
		// establishes the ordered migration seam without re-validating legacy values.
		return nil
	default:
		return fmt.Errorf("no migration registered from version %d", from)
	}
}

func rejectUnavailableComposition(source string, root *lua.LTable, version int, available map[string]fieldSchema) error {
	names := make([]string, 0, len(unavailableV2Fields))
	for name := range unavailableV2Fields {
		if root.RawGetString(name) != lua.LNil {
			if version == 2 {
				if _, ok := available[name]; ok {
					continue
				}
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil
	}
	name := names[0]
	if version == 1 {
		return documentError(source, name, "requires config_version = 2")
	}
	return documentError(source, name, "is reserved for a later Phase 2 slice and is not available yet")
}

func collectPresence(table *lua.LTable, schema fieldSchema, prefix string, out map[string]struct{}) {
	for _, child := range schema.children {
		value := table.RawGetString(child.name)
		if value == lua.LNil {
			continue
		}
		path := joinPath(prefix, child.name)
		out[path] = struct{}{}
		if child.kind == KindTable {
			if nested, ok := value.(*lua.LTable); ok {
				collectPresence(nested, child, path, out)
			}
		}
	}
}
