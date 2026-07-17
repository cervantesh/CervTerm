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
	KindTable      ValueKind = "table"
	KindString     ValueKind = "string"
	KindNumber     ValueKind = "number"
	KindInteger    ValueKind = "integer"
	KindBoolean    ValueKind = "boolean"
	KindStringList ValueKind = "string_list"
	KindStringMap  ValueKind = "string_map"
	KindKeyList    ValueKind = "key_list"
	KindEvents     ValueKind = "events"
)

type FieldMetadata struct {
	Path      string
	Kind      ValueKind
	Available bool
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
	return FromTable(base, document.Root)
}

type fieldSchema struct {
	name     string
	kind     ValueKind
	required bool
	children []fieldSchema
}

var rootSchema = fieldSchema{kind: KindTable, children: []fieldSchema{
	{name: "window", kind: KindTable, children: []fieldSchema{
		{name: "width", kind: KindInteger}, {name: "height", kind: KindInteger},
		{name: "padding_x", kind: KindInteger}, {name: "padding_y", kind: KindInteger},
		{name: "dynamic_title", kind: KindBoolean}, {name: "opacity", kind: KindNumber}, {name: "blur", kind: KindBoolean},
	}},
	{name: "font", kind: KindTable, children: []fieldSchema{
		{name: "family", kind: KindString}, {name: "size", kind: KindNumber}, {name: "ligatures", kind: KindBoolean},
	}},
	{name: "colors", kind: KindTable, children: []fieldSchema{
		{name: "foreground", kind: KindString}, {name: "background", kind: KindString},
		{name: "cursor", kind: KindString}, {name: "selection_background", kind: KindString},
	}},
	{name: "scrolling", kind: KindTable, children: []fieldSchema{
		{name: "history", kind: KindInteger}, {name: "wheel_multiplier", kind: KindInteger}, {name: "hide_cursor_when_scrolled", kind: KindBoolean},
	}},
	{name: "scrollbar", kind: KindTable, children: []fieldSchema{
		{name: "enabled", kind: KindBoolean}, {name: "reserved_width_px", kind: KindInteger},
		{name: "width_px", kind: KindInteger}, {name: "margin_px", kind: KindInteger},
		{name: "radius_px", kind: KindInteger}, {name: "min_thumb_px", kind: KindInteger},
		{name: "track_color", kind: KindString}, {name: "thumb_color", kind: KindString},
		{name: "thumb_hover_color", kind: KindString}, {name: "thumb_press_color", kind: KindString},
		{name: "auto_hide_delay_ms", kind: KindInteger}, {name: "fade_ms", kind: KindInteger},
		{name: "page_step", kind: KindNumber}, {name: "track_click", kind: KindString},
	}},
	{name: "cursor", kind: KindTable, children: []fieldSchema{
		{name: "shape", kind: KindString}, {name: "blink", kind: KindBoolean},
		{name: "blink_interval_ms", kind: KindInteger}, {name: "thickness", kind: KindNumber},
	}},
	{name: "clipboard", kind: KindTable, children: []fieldSchema{{name: "osc52", kind: KindString}}},
	{name: "render", kind: KindTable, children: []fieldSchema{
		{name: "bidi", kind: KindBoolean}, {name: "text_gamma", kind: KindNumber}, {name: "text_darken", kind: KindNumber},
		{name: "text_raster", kind: KindString}, {name: "stats_hotkey", kind: KindString},
		{name: "zoom_in_hotkey", kind: KindString}, {name: "zoom_out_hotkey", kind: KindString},
		{name: "zoom_reset_hotkey", kind: KindString}, {name: "vsync", kind: KindBoolean},
		{name: "redraw", kind: KindString}, {name: "damage", kind: KindString},
	}},
	{name: "shell", kind: KindTable, children: []fieldSchema{
		{name: "program", kind: KindString}, {name: "args", kind: KindStringList},
		{name: "working_directory", kind: KindString}, {name: "env", kind: KindStringMap},
	}},
	{name: "keys", kind: KindKeyList},
	{name: "events", kind: KindEvents},
}}

var unavailableV2Fields = map[string]ValueKind{
	"includes": KindStringList, "default_environment": KindString, "default_profile": KindString,
	"environments": KindTable, "profiles": KindTable,
}

func SchemaFields(version int) ([]FieldMetadata, error) {
	if version < MinimumSchemaVersion || version > CurrentSchemaVersion {
		return nil, fmt.Errorf("unsupported config schema version %d", version)
	}
	fields := []FieldMetadata{{Path: "config_version", Kind: KindInteger, Available: true}}
	var walk func(prefix string, schema fieldSchema)
	walk = func(prefix string, schema fieldSchema) {
		for _, child := range schema.children {
			path := child.name
			if prefix != "" {
				path = prefix + "." + child.name
			}
			fields = append(fields, FieldMetadata{Path: path, Kind: child.kind, Available: true})
			if child.kind == KindTable {
				walk(path, child)
			}
		}
	}
	walk("", rootSchema)
	if version == 2 {
		for _, name := range []string{"includes", "default_environment", "default_profile", "environments", "profiles"} {
			fields = append(fields, FieldMetadata{Path: name, Kind: unavailableV2Fields[name], Available: false})
		}
	}
	return fields, nil
}

func DecodeDocument(source string, root *lua.LTable) (Document, error) {
	return decodeDocument(source, root, nil)
}

func decodeDocument(source string, root *lua.LTable, available map[string]fieldSchema) (Document, error) {
	version, err := documentVersion(source, root)
	if err != nil {
		return Document{}, err
	}
	document := Document{
		Source: source, AuthoredVersion: version, Version: version,
		Root: root, Present: make(map[string]struct{}),
	}
	collectPresence(root, rootSchema, "", document.Present)
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
		if err := validateStrictTable(source, "", root, rootSchema, true, available); err != nil {
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
