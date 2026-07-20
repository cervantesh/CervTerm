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
	KindTable               ValueKind = "table"
	KindString              ValueKind = "string"
	KindNumber              ValueKind = "number"
	KindInteger             ValueKind = "integer"
	KindBoolean             ValueKind = "boolean"
	KindStringList          ValueKind = "string_list"
	KindStringMap           ValueKind = "string_map"
	KindFeatureMap          ValueKind = "feature_map"
	KindIndexedColorMap     ValueKind = "indexed_color_map"
	KindKeyList             ValueKind = "key_list"
	KindEvents              ValueKind = "events"
	KindDocumentMap         ValueKind = "document_map"
	KindDescriptorList      ValueKind = "descriptor_list"
	KindFontRuleList        ValueKind = "font_rule_list"
	KindColorSchemeMap      ValueKind = "color_scheme_map"
	KindBackgroundLayerList ValueKind = "background_layer_list"
	KindQuickSelectRuleList ValueKind = "quick_select_rule_list"
	KindLaunchTargetList    ValueKind = "launch_target_list"
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
	if window := tableField(document.Root, "window"); window != nil {
		if document.Has("window.padding_x") {
			cfg.Window.PaddingLeft = cfg.Window.PaddingX
			cfg.Window.PaddingRight = cfg.Window.PaddingX
		}
		if document.Has("window.padding_y") {
			cfg.Window.PaddingTop = cfg.Window.PaddingY
			cfg.Window.PaddingBottom = cfg.Window.PaddingY
		}
		if document.AuthoredVersion >= 2 {
			cfg.Window.PaddingLeft = intField(window, "padding_left", cfg.Window.PaddingLeft)
			cfg.Window.PaddingRight = intField(window, "padding_right", cfg.Window.PaddingRight)
			cfg.Window.PaddingTop = intField(window, "padding_top", cfg.Window.PaddingTop)
			cfg.Window.PaddingBottom = intField(window, "padding_bottom", cfg.Window.PaddingBottom)
			cfg.Window.TextOpacity = numberField(window, "text_opacity", cfg.Window.TextOpacity)
			cfg.Window.BackgroundOpacity = numberField(window, "background_opacity", cfg.Window.BackgroundOpacity)
			cfg.Window.InitialRows = intField(window, "initial_rows", cfg.Window.InitialRows)
			cfg.Window.InitialCols = intField(window, "initial_cols", cfg.Window.InitialCols)
			cfg.Window.Decorations = stringField(window, "decorations", cfg.Window.Decorations)
			cfg.Window.Titlebar = stringField(window, "titlebar", cfg.Window.Titlebar)
		}
	}
	if scrollbar := tableField(document.Root, "scrollbar"); scrollbar != nil {
		if document.AuthoredVersion >= 2 {
			if document.Has("scrollbar.mode") {
				cfg.Scrollbar.Mode = stringField(scrollbar, "mode", cfg.Scrollbar.Mode)
				cfg.Scrollbar.Enabled = cfg.Scrollbar.Mode != "never"
			} else if document.Has("scrollbar.enabled") {
				if cfg.Scrollbar.Enabled {
					cfg.Scrollbar.Mode = "scrolling"
				} else {
					cfg.Scrollbar.Mode = "never"
				}
			}
			cfg.Scrollbar.StableGutter = boolField(scrollbar, "stable_gutter", cfg.Scrollbar.StableGutter)
			cfg.Scrollbar.AnimationFPS = intField(scrollbar, "animation_fps", cfg.Scrollbar.AnimationFPS)
		} else {
			if cfg.Scrollbar.Enabled {
				cfg.Scrollbar.Mode = "scrolling"
			} else {
				cfg.Scrollbar.Mode = "never"
			}
			cfg.Scrollbar.StableGutter = true
			cfg.Scrollbar.AnimationFPS = base.Scrollbar.AnimationFPS
		}
	}
	if document.AuthoredVersion < 2 {
		cfg.TabBar = base.TabBar
	}
	if document.AuthoredVersion < 2 {
		cfg.Window.TextOpacity = base.Window.TextOpacity
		cfg.Window.BackgroundOpacity = base.Window.BackgroundOpacity
		cfg.Render.MaxFPS = base.Render.MaxFPS
		cfg.Window.InitialRows = base.Window.InitialRows
		cfg.Window.InitialCols = base.Window.InitialCols
		cfg.Window.Decorations = base.Window.Decorations
		cfg.Window.Titlebar = base.Window.Titlebar
	}
	if document.AuthoredVersion >= 2 {
		cfg.ColorScheme = stringField(document.Root, "color_scheme", cfg.ColorScheme)
		if background := tableField(document.Root, "background"); background != nil {
			cfg.Background.Layers = backgroundLayerListField(background, "layers", cfg.Background.Layers)
		}
		if font := tableField(document.Root, "font"); font != nil {
			cfg.Font.Descriptors = descriptorListField(font, "descriptors", cfg.Font.Descriptors)
			cfg.Font.Fallback = descriptorListField(font, "fallback", cfg.Font.Fallback)
			cfg.Font.Rules = fontRuleListField(font, "rules", cfg.Font.Rules)
			cfg.Font.Features = integerMapField(font, "features", cfg.Font.Features)
			cfg.Font.LineHeight = numberField(font, "line_height", cfg.Font.LineHeight)
			cfg.Font.CellWidth = numberField(font, "cell_width", cfg.Font.CellWidth)
			cfg.Font.BaselineOffset = numberField(font, "baseline_offset", cfg.Font.BaselineOffset)
			cfg.Font.GlyphOffsetX = numberField(font, "glyph_offset_x", cfg.Font.GlyphOffsetX)
			cfg.Font.GlyphOffsetY = numberField(font, "glyph_offset_y", cfg.Font.GlyphOffsetY)
		}
		if render := tableField(document.Root, "render"); render != nil {
			cfg.Render.MaxFPS = intField(render, "max_fps", cfg.Render.MaxFPS)
		}
		if imeConfig := tableField(document.Root, "ime"); imeConfig != nil {
			cfg.IME.Enabled = boolField(imeConfig, "enabled", cfg.IME.Enabled)
		}
	}
	if quick := tableField(document.Root, "quick_select"); quick != nil {
		cfg.QuickSelect.Rules = quickSelectRuleListField(quick, "rules", cfg.QuickSelect.Rules)
		cfg.QuickSelect.Compiled, _ = PrepareQuickSelect(cfg.QuickSelect.Rules)
	}
	cfg.LaunchMenu = launchTargetListField(document.Root, "launch_menu", cfg.LaunchMenu)
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
		{name: "initial_rows", kind: KindInteger, apply: ApplyNewWindow}, {name: "initial_cols", kind: KindInteger, apply: ApplyNewWindow},
		{name: "decorations", kind: KindString, apply: ApplyWindowRecreate}, {name: "titlebar", kind: KindString, apply: ApplyWindowRecreate},
		{name: "padding_x", kind: KindInteger, apply: ApplyRestart}, {name: "padding_y", kind: KindInteger, apply: ApplyRestart},
		{name: "padding_left", kind: KindInteger, apply: ApplyRestart}, {name: "padding_right", kind: KindInteger, apply: ApplyRestart},
		{name: "padding_top", kind: KindInteger, apply: ApplyRestart}, {name: "padding_bottom", kind: KindInteger, apply: ApplyRestart},
		{name: "dynamic_title", kind: KindBoolean, apply: ApplyRestart},
		{name: "opacity", kind: KindNumber, apply: ApplyLive, runtimeOverride: true},
		{name: "text_opacity", kind: KindNumber, apply: ApplyLive, runtimeOverride: true},
		{name: "background_opacity", kind: KindNumber, apply: ApplyLive, runtimeOverride: true},
		{name: "blur", kind: KindBoolean, apply: ApplyLive, runtimeOverride: true},
	}},
	{name: "layout_persistence", kind: KindTable, apply: ApplyRestart, children: []fieldSchema{
		{name: "enabled", kind: KindBoolean}, {name: "path", kind: KindString, sensitive: true},
	}},
	{name: "font", kind: KindTable, apply: ApplyRestart, children: []fieldSchema{
		{name: "family", kind: KindString}, {name: "descriptors", kind: KindDescriptorList}, {name: "fallback", kind: KindDescriptorList}, {name: "rules", kind: KindFontRuleList},
		{name: "size", kind: KindNumber}, {name: "ligatures", kind: KindBoolean}, {name: "features", kind: KindFeatureMap},
		{name: "line_height", kind: KindNumber}, {name: "cell_width", kind: KindNumber}, {name: "baseline_offset", kind: KindNumber},
		{name: "glyph_offset_x", kind: KindNumber}, {name: "glyph_offset_y", kind: KindNumber},
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
	{name: "background", kind: KindTable, apply: ApplyLive, children: []fieldSchema{
		{name: "layers", kind: KindBackgroundLayerList},
	}},
	{name: "scrolling", kind: KindTable, apply: ApplyLive, runtimeOverride: true, children: []fieldSchema{
		{name: "history", kind: KindInteger}, {name: "wheel_multiplier", kind: KindInteger}, {name: "hide_cursor_when_scrolled", kind: KindBoolean},
	}},
	{name: "scrollbar", kind: KindTable, apply: ApplyLive, runtimeOverride: true, children: []fieldSchema{
		{name: "enabled", kind: KindBoolean}, {name: "mode", kind: KindString}, {name: "stable_gutter", kind: KindBoolean}, {name: "animation_fps", kind: KindInteger},
		{name: "reserved_width_px", kind: KindInteger}, {name: "width_px", kind: KindInteger}, {name: "margin_px", kind: KindInteger},
		{name: "radius_px", kind: KindInteger}, {name: "min_thumb_px", kind: KindInteger},
		{name: "track_color", kind: KindString}, {name: "thumb_color", kind: KindString},
		{name: "thumb_hover_color", kind: KindString}, {name: "thumb_press_color", kind: KindString},
		{name: "auto_hide_delay_ms", kind: KindInteger}, {name: "fade_ms", kind: KindInteger},
		{name: "page_step", kind: KindNumber}, {name: "track_click", kind: KindString},
	}},
	{name: "tab_bar", kind: KindTable, apply: ApplyLive, runtimeOverride: true, children: []fieldSchema{
		{name: "mode", kind: KindString}, {name: "position", kind: KindString}, {name: "height_px", kind: KindInteger},
		{name: "min_width_px", kind: KindInteger}, {name: "max_width_px", kind: KindInteger}, {name: "padding_x", kind: KindInteger},
		{name: "show_new_button", kind: KindBoolean}, {name: "show_close_button", kind: KindBoolean},
	}},
	{name: "cursor", kind: KindTable, apply: ApplyLive, children: []fieldSchema{
		{name: "shape", kind: KindString}, {name: "blink", kind: KindBoolean},
		{name: "blink_interval_ms", kind: KindInteger}, {name: "thickness", kind: KindNumber},
	}},
	{name: "clipboard", kind: KindTable, apply: ApplyRestart, children: []fieldSchema{{name: "osc52", kind: KindString}}},
	{name: "ime", kind: KindTable, apply: ApplyRestart, children: []fieldSchema{{name: "enabled", kind: KindBoolean}}},
	{name: "bell", kind: KindTable, apply: ApplyLive, children: []fieldSchema{
		{name: "mode", kind: KindString}, {name: "focus", kind: KindString},
		{name: "throttle_ms", kind: KindInteger}, {name: "visual_duration_ms", kind: KindInteger},
	}},
	{name: "notification", kind: KindTable, apply: ApplyLive, children: []fieldSchema{
		{name: "enabled", kind: KindBoolean}, {name: "focus", kind: KindString}, {name: "rate_limit_ms", kind: KindInteger},
	}},
	{name: "render", kind: KindTable, apply: ApplyRestart, children: []fieldSchema{
		{name: "bidi", kind: KindBoolean}, {name: "text_gamma", kind: KindNumber}, {name: "text_darken", kind: KindNumber},
		{name: "text_raster", kind: KindString}, {name: "stats_hotkey", kind: KindString},
		{name: "zoom_in_hotkey", kind: KindString}, {name: "zoom_out_hotkey", kind: KindString},
		{name: "zoom_reset_hotkey", kind: KindString}, {name: "vsync", kind: KindBoolean},
		{name: "max_fps", kind: KindInteger, apply: ApplyLive, runtimeOverride: true},
		{name: "redraw", kind: KindString}, {name: "damage", kind: KindString},
	}},
	{name: "shell", kind: KindTable, apply: ApplyNewPane, children: []fieldSchema{
		{name: "program", kind: KindString}, {name: "args", kind: KindStringList},
		{name: "working_directory", kind: KindString}, {name: "env", kind: KindStringMap, sensitive: true},
	}},
	{name: "launch_menu", kind: KindLaunchTargetList, apply: ApplyLive},
	{name: "quick_select", kind: KindTable, apply: ApplyLive, children: []fieldSchema{
		{name: "rules", kind: KindQuickSelectRuleList},
	}},
	{name: "keys", kind: KindKeyList, apply: ApplyLive},
	{name: "events", kind: KindEvents, apply: ApplyLive},
}}

var unavailableV2Fields = map[string]ValueKind{
	"includes": KindStringList, "default_environment": KindString, "default_profile": KindString,
	"environments": KindDocumentMap, "profiles": KindDocumentMap, "color_schemes": KindColorSchemeMap,
}

func isV2OnlyPath(path string) bool {
	switch path {
	case "layout_persistence", "layout_persistence.enabled", "layout_persistence.path",
		"window.padding_left", "window.padding_right", "window.padding_top", "window.padding_bottom",
		"window.text_opacity", "window.background_opacity",
		"window.initial_rows", "window.initial_cols", "window.decorations", "window.titlebar",
		"background.layers", "quick_select.rules", "launch_menu",
		"scrollbar.mode", "scrollbar.stable_gutter", "scrollbar.animation_fps", "render.max_fps",
		"tab_bar.mode", "tab_bar.position", "tab_bar.height_px", "tab_bar.min_width_px", "tab_bar.max_width_px", "tab_bar.padding_x", "tab_bar.show_new_button", "tab_bar.show_close_button",
		"bell", "bell.mode", "bell.focus", "bell.throttle_ms", "bell.visual_duration_ms",
		"notification", "notification.enabled", "notification.focus", "notification.rate_limit_ms",
		"ime", "ime.enabled",
		"font.descriptors", "font.fallback", "font.rules", "font.features", "font.line_height", "font.cell_width", "font.baseline_offset", "font.glyph_offset_x", "font.glyph_offset_y":
		return true
	default:
		return false
	}
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
			if version == 1 && isV2OnlyPath(path) {
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
	if version == 1 && root.RawGetString("layout_persistence") != lua.LNil {
		return Document{}, documentError(source, "layout_persistence", "requires config_version = 2")
	}
	if version == 1 && root.RawGetString("bell") != lua.LNil {
		return Document{}, documentError(source, "bell", "requires config_version = 2")
	}
	if version == 1 && root.RawGetString("notification") != lua.LNil {
		return Document{}, documentError(source, "notification", "requires config_version = 2")
	}
	if version == 1 && root.RawGetString("ime") != lua.LNil {
		return Document{}, documentError(source, "ime", "requires config_version = 2")
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
		delete(document.Present, "window.padding_left")
		delete(document.Present, "window.padding_right")
		delete(document.Present, "window.padding_top")
		delete(document.Present, "window.padding_bottom")
		delete(document.Present, "window.text_opacity")
		delete(document.Present, "window.background_opacity")
		delete(document.Present, "window.initial_rows")
		delete(document.Present, "window.initial_cols")
		delete(document.Present, "window.decorations")
		delete(document.Present, "window.titlebar")
		delete(document.Present, "layout_persistence")
		delete(document.Present, "layout_persistence.enabled")
		delete(document.Present, "layout_persistence.path")
		delete(document.Present, "background.layers")
		delete(document.Present, "scrollbar.mode")
		delete(document.Present, "scrollbar.stable_gutter")
		delete(document.Present, "scrollbar.animation_fps")
		delete(document.Present, "render.max_fps")
		delete(document.Present, "font.descriptors")
		delete(document.Present, "font.fallback")
		delete(document.Present, "font.rules")
		delete(document.Present, "font.features")
		delete(document.Present, "font.line_height")
		delete(document.Present, "font.cell_width")
		delete(document.Present, "font.baseline_offset")
		delete(document.Present, "font.glyph_offset_x")
		delete(document.Present, "font.glyph_offset_y")
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
