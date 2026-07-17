package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// RuntimeOverrideAllowed reports the schema-owned scoped patch capability.
func RuntimeOverrideAllowed(path string) bool {
	if path == "" || strings.TrimSpace(path) != path {
		return false
	}
	current := rootSchema
	apply := ApplyScope("")
	allowed := false
	sensitive := false
	parts := strings.Split(path, ".")
	for index, part := range parts {
		var child *fieldSchema
		for childIndex := range current.children {
			if current.children[childIndex].name == part {
				child = &current.children[childIndex]
				break
			}
		}
		if child == nil {
			return false
		}
		if child.apply != "" {
			apply = child.apply
		}
		allowed = allowed || child.runtimeOverride
		sensitive = sensitive || child.sensitive
		if index == len(parts)-1 {
			return child.kind != KindTable && apply == ApplyLive && allowed && !sensitive
		}
		if child.kind != KindTable {
			return false
		}
		current = *child
	}
	return false
}

// ConfigScopeID is an opaque process-local owner for runtime configuration patches.
type ConfigScopeID struct{ value uint64 }

func (id ConfigScopeID) Valid() bool { return id.value != 0 }
func (id ConfigScopeID) String() string {
	if !id.Valid() {
		return "config-scope-invalid"
	}
	return fmt.Sprintf("config-scope-%d", id.value)
}

// RuntimeOverride is the raw typed patch form shared with CLI coercion rules.
type RuntimeOverride struct {
	Path  string
	Value string
}

// RuntimeOverrideRecord is value-free diagnostic/provenance evidence.
type RuntimeOverrideRecord struct {
	Path  string
	Scope ConfigScopeID
}

// RuntimeOverrideProvenance overlays runtime winners onto composed provenance,
// retaining the complete low-to-high overwritten chain without values.
func RuntimeOverrideProvenance(base []ProvenanceRecord, records []RuntimeOverrideRecord) []ProvenanceRecord {
	byPath := make(map[string]ProvenanceRecord, len(base)+len(records))
	for _, record := range base {
		byPath[record.Path] = cloneProvenanceRecord(record)
	}
	for _, runtime := range records {
		prior, exists := byPath[runtime.Path]
		overwritten := append([]ProvenanceOrigin(nil), prior.Overwritten...)
		if exists && prior.Winner.Layer != "" {
			overwritten = append(overwritten, prior.Winner)
		}
		byPath[runtime.Path] = ProvenanceRecord{
			Path: runtime.Path, Sensitive: prior.Sensitive, Overwritten: overwritten,
			Winner: ProvenanceOrigin{
				Layer: LayerRuntime, Name: runtime.Scope.String(),
				ConfigScopeID: runtime.Scope, HasConfigScopeID: true,
			},
		}
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	provenance := make([]ProvenanceRecord, 0, len(paths))
	for _, path := range paths {
		provenance = append(provenance, cloneProvenanceRecord(byPath[path]))
	}
	return provenance
}

type runtimeScopePatch struct {
	values   Config
	paths    map[string]struct{}
	revision uint64
}

// RuntimeScopes owns typed patches for live process-local config scopes.
type RuntimeScopes struct {
	next   uint64
	scopes map[ConfigScopeID]runtimeScopePatch
}

func (s *RuntimeScopes) NewScope() ConfigScopeID {
	if s.scopes == nil {
		s.scopes = make(map[ConfigScopeID]runtimeScopePatch)
	}
	s.next++
	id := ConfigScopeID{value: s.next}
	s.scopes[id] = runtimeScopePatch{paths: make(map[string]struct{})}
	return id
}

func (s *RuntimeScopes) CloseScope(id ConfigScopeID) bool {
	if s == nil || !id.Valid() {
		return false
	}
	if _, ok := s.scopes[id]; !ok {
		return false
	}
	delete(s.scopes, id)
	return true
}

func (s *RuntimeScopes) patch(id ConfigScopeID) (runtimeScopePatch, error) {
	if s == nil || !id.Valid() {
		return runtimeScopePatch{}, fmt.Errorf("configuration scope is closed")
	}
	patch, ok := s.scopes[id]
	if !ok {
		return runtimeScopePatch{}, fmt.Errorf("configuration scope %s is closed", id)
	}
	return patch, nil
}

// RuntimePatchTransaction is allocation-complete and may be committed only to
// the RuntimeScopes instance that prepared it.
type RuntimePatchTransaction struct {
	owner            *RuntimeScopes
	scope            ConfigScopeID
	expectedRevision uint64
	patch            runtimeScopePatch
	desired          Config
	records          []RuntimeOverrideRecord
	committed        bool
}

func (t *RuntimePatchTransaction) Desired() Config { return t.desired.Clone() }
func (t *RuntimePatchTransaction) Records() []RuntimeOverrideRecord {
	return append([]RuntimeOverrideRecord(nil), t.records...)
}

// Commit is mechanically infallible under the serialized main-thread contract.
func (t *RuntimePatchTransaction) Commit() {
	if t == nil || t.committed || t.owner == nil {
		return
	}
	active, ok := t.owner.scopes[t.scope]
	if !ok || active.revision != t.expectedRevision {
		panic("runtime config scope transaction committed after ownership changed")
	}
	t.patch.revision = active.revision + 1
	t.owner.scopes[t.scope] = t.patch
	t.committed = true
}

func cloneRuntimePatch(patch runtimeScopePatch) runtimeScopePatch {
	clone := runtimeScopePatch{values: patch.values.Clone(), paths: make(map[string]struct{}, len(patch.paths)), revision: patch.revision}
	for path := range patch.paths {
		clone.paths[path] = struct{}{}
	}
	return clone
}

// ProposeConfig adapts existing typed setter Config values into the same raw
// path/value decoder used by CLI overrides.
func (s *RuntimeScopes) ProposeConfig(id ConfigScopeID, composed, effective, next Config) (*RuntimePatchTransaction, error) {
	changes := DiffConfig(next, effective)
	overrides := make([]RuntimeOverride, 0, len(changes))
	for _, change := range changes {
		raw, err := runtimeOverrideRawValue(next, change.Path)
		if err != nil {
			return nil, err
		}
		overrides = append(overrides, RuntimeOverride{Path: change.Path, Value: raw})
	}
	return s.ProposeOverrides(id, composed, overrides)
}

// ProposeOverrides decodes and validates one ordered runtime patch transaction.
func (s *RuntimeScopes) ProposeOverrides(id ConfigScopeID, composed Config, overrides []RuntimeOverride) (*RuntimePatchTransaction, error) {
	active, err := s.patch(id)
	if err != nil {
		return nil, err
	}
	proposed := cloneRuntimePatch(active)
	for _, override := range overrides {
		if !RuntimeOverrideAllowed(override.Path) {
			return nil, fmt.Errorf("runtime configuration path %q does not permit scoped override", override.Path)
		}
		proposed.values, err = decodeRuntimeOverride(proposed.values, override)
		if err != nil {
			return nil, fmt.Errorf("runtime configuration path %q: %w", override.Path, err)
		}
		proposed.paths[override.Path] = struct{}{}
	}
	desired := applyRuntimePatch(composed, proposed)
	if err := desired.Validate(); err != nil {
		return nil, err
	}
	return &RuntimePatchTransaction{
		owner: s, scope: id, expectedRevision: active.revision, patch: proposed,
		desired: desired, records: runtimeRecords(id, proposed),
	}, nil
}

func decodeRuntimeOverride(base Config, override RuntimeOverride) (Config, error) {
	resolved, err := resolveCLIOverridePath(override.Path)
	if err != nil {
		return Config{}, err
	}
	state := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer state.Close()
	value, _, err := decodeCLIOverrideValue(state, resolved, override.Value)
	if err != nil {
		return Config{}, err
	}
	root := state.NewTable()
	target := root
	for _, part := range resolved.parts[:len(resolved.parts)-1] {
		nested := state.NewTable()
		target.RawSetString(part, nested)
		target = nested
	}
	target.RawSetString(resolved.parts[len(resolved.parts)-1], value)
	return FromTable(base, root), nil
}

func runtimeOverrideRawValue(source Config, path string) (string, error) {
	var value any
	switch path {
	case "window.opacity":
		value = source.Window.Opacity
	case "window.blur":
		value = source.Window.Blur
	case "colors.background":
		value = source.Colors.Background
	case "scrolling.history":
		value = source.Scrolling.History
	case "scrolling.wheel_multiplier":
		value = source.Scrolling.WheelMultiplier
	case "scrolling.hide_cursor_when_scrolled":
		value = source.Scrolling.HideCursorWhenScrolled
	case "scrollbar.enabled":
		value = source.Scrollbar.Enabled
	case "scrollbar.reserved_width_px":
		value = source.Scrollbar.ReservedWidthPX
	case "scrollbar.width_px":
		value = source.Scrollbar.WidthPX
	case "scrollbar.margin_px":
		value = source.Scrollbar.MarginPX
	case "scrollbar.radius_px":
		value = source.Scrollbar.RadiusPX
	case "scrollbar.min_thumb_px":
		value = source.Scrollbar.MinThumbPX
	case "scrollbar.track_color":
		value = source.Scrollbar.TrackColor
	case "scrollbar.thumb_color":
		value = source.Scrollbar.ThumbColor
	case "scrollbar.thumb_hover_color":
		value = source.Scrollbar.ThumbHoverColor
	case "scrollbar.thumb_press_color":
		value = source.Scrollbar.ThumbPressColor
	case "scrollbar.auto_hide_delay_ms":
		value = source.Scrollbar.AutoHideDelayMS
	case "scrollbar.fade_ms":
		value = source.Scrollbar.FadeMS
	case "scrollbar.page_step":
		value = source.Scrollbar.PageStep
	case "scrollbar.track_click":
		value = source.Scrollbar.TrackClick
	default:
		return "", fmt.Errorf("runtime configuration path %q does not permit scoped override", path)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ProposeClear removes selected owned paths, or every path when none are given.
func (s *RuntimeScopes) ProposeClear(id ConfigScopeID, composed Config, paths ...string) (*RuntimePatchTransaction, error) {
	active, err := s.patch(id)
	if err != nil {
		return nil, err
	}
	proposed := cloneRuntimePatch(active)
	if len(paths) == 0 {
		proposed.paths = make(map[string]struct{})
	} else {
		for _, path := range paths {
			if !RuntimeOverrideAllowed(path) {
				return nil, fmt.Errorf("runtime configuration path %q does not permit scoped override", path)
			}
			delete(proposed.paths, path)
		}
	}
	desired := applyRuntimePatch(composed, proposed)
	if err := desired.Validate(); err != nil {
		return nil, err
	}
	return &RuntimePatchTransaction{
		owner: s, scope: id, expectedRevision: active.revision, patch: proposed,
		desired: desired, records: runtimeRecords(id, proposed),
	}, nil
}

// Apply revalidates an existing scope against a new composed candidate.
func (s *RuntimeScopes) Apply(id ConfigScopeID, composed Config) (Config, []RuntimeOverrideRecord, error) {
	patch, err := s.patch(id)
	if err != nil {
		return Config{}, nil, err
	}
	desired := applyRuntimePatch(composed, patch)
	if err := desired.Validate(); err != nil {
		return Config{}, nil, err
	}
	return desired, runtimeRecords(id, patch), nil
}

func runtimeRecords(id ConfigScopeID, patch runtimeScopePatch) []RuntimeOverrideRecord {
	paths := make([]string, 0, len(patch.paths))
	for path := range patch.paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	records := make([]RuntimeOverrideRecord, 0, len(paths))
	for _, path := range paths {
		records = append(records, RuntimeOverrideRecord{Path: path, Scope: id})
	}
	return records
}

func applyRuntimePatch(base Config, patch runtimeScopePatch) Config {
	result := base.Clone()
	paths := make([]string, 0, len(patch.paths))
	for path := range patch.paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		applyRuntimePath(&result, patch.values, path)
	}
	return result
}

func applyRuntimePath(target *Config, source Config, path string) {
	switch path {
	case "window.opacity":
		target.Window.Opacity = source.Window.Opacity
	case "window.blur":
		target.Window.Blur = source.Window.Blur
	case "colors.background":
		target.Colors.Background = source.Colors.Background
	case "scrolling.history":
		target.Scrolling.History = source.Scrolling.History
	case "scrolling.wheel_multiplier":
		target.Scrolling.WheelMultiplier = source.Scrolling.WheelMultiplier
	case "scrolling.hide_cursor_when_scrolled":
		target.Scrolling.HideCursorWhenScrolled = source.Scrolling.HideCursorWhenScrolled
	case "scrollbar.enabled":
		target.Scrollbar.Enabled = source.Scrollbar.Enabled
	case "scrollbar.reserved_width_px":
		target.Scrollbar.ReservedWidthPX = source.Scrollbar.ReservedWidthPX
	case "scrollbar.width_px":
		target.Scrollbar.WidthPX = source.Scrollbar.WidthPX
	case "scrollbar.margin_px":
		target.Scrollbar.MarginPX = source.Scrollbar.MarginPX
	case "scrollbar.radius_px":
		target.Scrollbar.RadiusPX = source.Scrollbar.RadiusPX
	case "scrollbar.min_thumb_px":
		target.Scrollbar.MinThumbPX = source.Scrollbar.MinThumbPX
	case "scrollbar.track_color":
		target.Scrollbar.TrackColor = source.Scrollbar.TrackColor
	case "scrollbar.thumb_color":
		target.Scrollbar.ThumbColor = source.Scrollbar.ThumbColor
	case "scrollbar.thumb_hover_color":
		target.Scrollbar.ThumbHoverColor = source.Scrollbar.ThumbHoverColor
	case "scrollbar.thumb_press_color":
		target.Scrollbar.ThumbPressColor = source.Scrollbar.ThumbPressColor
	case "scrollbar.auto_hide_delay_ms":
		target.Scrollbar.AutoHideDelayMS = source.Scrollbar.AutoHideDelayMS
	case "scrollbar.fade_ms":
		target.Scrollbar.FadeMS = source.Scrollbar.FadeMS
	case "scrollbar.page_step":
		target.Scrollbar.PageStep = source.Scrollbar.PageStep
	case "scrollbar.track_click":
		target.Scrollbar.TrackClick = source.Scrollbar.TrackClick
	default:
		panic("applyRuntimePath called for unsupported path: " + path)
	}
}
