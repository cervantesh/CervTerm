package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeScopePatchSurvivesReloadAndClears(t *testing.T) {
	base := Defaults()
	base.Colors.Background = "#080B12"
	var scopes RuntimeScopes
	scope := scopes.NewScope()
	if !scope.Valid() || scope.String() == "" {
		t.Fatalf("scope = %#v", scope)
	}

	next := base.Clone()
	next.Window.Opacity = 0.8
	next.Scrolling.History = 4000
	transaction, err := scopes.ProposeConfig(scope, base, base, next)
	if err != nil {
		t.Fatal(err)
	}
	if got := transaction.Desired(); got.Window.Opacity != 0.8 || got.Scrolling.History != 4000 {
		t.Fatalf("transaction desired = %#v", got)
	}
	wantRecords := []RuntimeOverrideRecord{{Path: "scrolling.history", Scope: scope}, {Path: "window.opacity", Scope: scope}}
	if got := transaction.Records(); !reflect.DeepEqual(got, wantRecords) {
		t.Fatalf("records = %#v, want %#v", got, wantRecords)
	}
	baseProvenance := []ProvenanceRecord{
		{Path: "scrolling.history", Winner: ProvenanceOrigin{Layer: LayerPrimary, Name: "config.lua"}},
		{Path: "window.opacity", Winner: ProvenanceOrigin{Layer: LayerInclude, Name: "base.lua"}, Overwritten: []ProvenanceOrigin{{Layer: LayerDefaults, Name: "defaults"}}},
	}
	provenance := RuntimeOverrideProvenance(baseProvenance, wantRecords)
	if len(provenance) != 2 || provenance[0].Winner.Layer != LayerRuntime || !provenance[0].Winner.HasConfigScopeID || provenance[0].Winner.ConfigScopeID != scope || len(provenance[0].Overwritten) != 1 || len(provenance[1].Overwritten) != 2 {
		t.Fatalf("runtime provenance = %#v", provenance)
	}
	transaction.Commit()

	reloaded := base.Clone()
	reloaded.Colors.Foreground = "#112233"
	reloaded.Window.Opacity = 0.95
	desired, records, err := scopes.Apply(scope, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if desired.Window.Opacity != 0.8 || desired.Scrolling.History != 4000 || desired.Colors.Foreground != "#112233" {
		t.Fatalf("reapplied desired = %#v", desired)
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("reapplied records = %#v", records)
	}

	clear, err := scopes.ProposeClear(scope, reloaded, "window.opacity")
	if err != nil {
		t.Fatal(err)
	}
	if clear.Desired().Window.Opacity != 0.95 || clear.Desired().Scrolling.History != 4000 {
		t.Fatalf("partial clear desired = %#v", clear.Desired())
	}
	clear.Commit()
	clearAll, err := scopes.ProposeClear(scope, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if len(clearAll.Records()) != 0 || !reflect.DeepEqual(clearAll.Desired(), reloaded) {
		t.Fatalf("clear all = desired %#v records %#v", clearAll.Desired(), clearAll.Records())
	}
	clearAll.Commit()
}

func TestRuntimeScopeRejectsUnsupportedAndInvalidTransactions(t *testing.T) {
	base := Defaults()
	base.Colors.Background = "#080B12"
	var scopes RuntimeScopes
	scope := scopes.NewScope()

	unsupported := base.Clone()
	unsupported.Colors.Foreground = "#010203"
	if _, err := scopes.ProposeConfig(scope, base, base, unsupported); err == nil || !strings.Contains(err.Error(), "does not permit") {
		t.Fatalf("unsupported error = %v", err)
	}

	next := base.Clone()
	next.Window.Opacity = 0.8
	valid, err := scopes.ProposeConfig(scope, base, base, next)
	if err != nil {
		t.Fatal(err)
	}
	valid.Commit()
	invalidReload := base.Clone()
	invalidReload.Colors.Background = "#01020380"
	if _, _, err := scopes.Apply(scope, invalidReload); err == nil || !strings.Contains(err.Error(), "cannot be enabled together") {
		t.Fatalf("invalid reapply error = %v", err)
	}
	stillApplied, _, err := scopes.Apply(scope, base)
	if err != nil || stillApplied.Window.Opacity != 0.8 {
		t.Fatalf("failed reapply mutated scope: %#v err=%v", stillApplied, err)
	}
}

func TestRuntimeOverridesUseSharedTypedDecoder(t *testing.T) {
	base := Defaults()
	base.Colors.Background = "#080B12"
	var scopes RuntimeScopes
	scope := scopes.NewScope()
	transaction, err := scopes.ProposeOverrides(scope, base, []RuntimeOverride{{Path: "window.opacity", Value: "0.8"}, {Path: "scrolling.history", Value: "4000"}})
	if err != nil {
		t.Fatal(err)
	}
	if transaction.Desired().Window.Opacity != 0.8 || transaction.Desired().Scrolling.History != 4000 {
		t.Fatalf("decoded runtime desired = %#v", transaction.Desired())
	}
	if _, err := scopes.ProposeOverrides(scope, base, []RuntimeOverride{{Path: "window.opacity", Value: "not-json"}}); err == nil || !strings.Contains(err.Error(), "JSON number") {
		t.Fatalf("runtime decoder error = %v", err)
	}
	if _, err := scopes.ProposeOverrides(scope, base, []RuntimeOverride{{Path: "shell.env", Value: `{\"TOKEN\":\"secret\"}`}}); err == nil || !strings.Contains(err.Error(), "does not permit") {
		t.Fatalf("sensitive runtime override error = %v", err)
	}
}

func TestRuntimeScopeLifecycleAndLastSuccessfulTransactionWins(t *testing.T) {
	base := Defaults()
	base.Colors.Background = "#080B12"
	var scopes RuntimeScopes
	scope := scopes.NewScope()

	first := base.Clone()
	first.Window.Opacity = 0.9
	one, err := scopes.ProposeConfig(scope, base, base, first)
	if err != nil {
		t.Fatal(err)
	}
	one.Commit()
	secondEffective, _, err := scopes.Apply(scope, base)
	if err != nil {
		t.Fatal(err)
	}
	second := secondEffective.Clone()
	second.Window.Opacity = 0.8
	two, err := scopes.ProposeConfig(scope, base, secondEffective, second)
	if err != nil {
		t.Fatal(err)
	}
	two.Commit()
	got, _, err := scopes.Apply(scope, base)
	if err != nil || got.Window.Opacity != 0.8 {
		t.Fatalf("last transaction desired = %#v err=%v", got, err)
	}

	if !scopes.CloseScope(scope) || scopes.CloseScope(scope) {
		t.Fatal("scope close lifecycle mismatch")
	}
	if _, _, err := scopes.Apply(scope, base); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("closed scope apply error = %v", err)
	}
	if _, err := scopes.ProposeConfig(scope, base, base, first); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("closed scope mutation error = %v", err)
	}
}

func TestRuntimeOverrideSchemaCapabilitiesMatchSetterSurface(t *testing.T) {
	allowed := []string{
		"window.opacity", "window.blur", "colors.background",
		"scrolling.history", "scrolling.wheel_multiplier", "scrolling.hide_cursor_when_scrolled",
		"scrollbar.enabled", "scrollbar.reserved_width_px", "scrollbar.width_px", "scrollbar.margin_px",
		"scrollbar.radius_px", "scrollbar.min_thumb_px", "scrollbar.track_color", "scrollbar.thumb_color",
		"scrollbar.thumb_hover_color", "scrollbar.thumb_press_color", "scrollbar.auto_hide_delay_ms",
		"scrollbar.fade_ms", "scrollbar.page_step", "scrollbar.track_click",
		"tab_bar.mode", "tab_bar.position", "tab_bar.height_px", "tab_bar.min_width_px",
		"tab_bar.max_width_px", "tab_bar.padding_x", "tab_bar.show_new_button", "tab_bar.show_close_button",
	}
	fields, err := SchemaFields(CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	metadata := make(map[string]FieldMetadata, len(fields))
	for _, field := range fields {
		metadata[field.Path] = field
	}
	for _, path := range allowed {
		if !RuntimeOverrideAllowed(path) {
			t.Fatalf("runtime setter path %q is not allowed", path)
		}
		if !metadata[path].RuntimeOverride {
			t.Fatalf("schema metadata omitted runtime capability for %q", path)
		}
	}
	for _, path := range []string{"colors.foreground", "cursor.shape", "shell.program", "keys", "events", "includes", "shell.env"} {
		if RuntimeOverrideAllowed(path) {
			t.Fatalf("unsupported path %q permits runtime override", path)
		}
		if metadata[path].RuntimeOverride {
			t.Fatalf("schema metadata exposed unsupported runtime capability for %q", path)
		}
	}
}
