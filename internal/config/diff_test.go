package config

import (
	"reflect"
	"testing"
)

func fullyDifferentConfig(base Config) Config {
	value := base.Clone()
	value.Window.Width++
	value.Window.Height++
	value.Window.PaddingX++
	value.Window.PaddingY++
	value.Window.DynamicTitle = !value.Window.DynamicTitle
	value.Window.Opacity = 0.75
	value.Window.Blur = !value.Window.Blur
	value.Font.Family += " Different"
	value.Font.Size++
	value.Font.Ligatures = !value.Font.Ligatures
	value.Colors.Foreground = "#010101"
	value.Colors.Background = "#020202"
	value.Colors.Cursor = "#030303"
	value.Colors.SelectionBackground = "#040404"
	value.Colors.ANSI[0] = "#111111"
	value.Scrolling.History++
	value.Scrolling.WheelMultiplier++
	value.Scrolling.HideCursorWhenScrolled = !value.Scrolling.HideCursorWhenScrolled
	value.Scrollbar.Enabled = !value.Scrollbar.Enabled
	value.Scrollbar.ReservedWidthPX++
	value.Scrollbar.WidthPX++
	value.Scrollbar.MarginPX++
	value.Scrollbar.RadiusPX++
	value.Scrollbar.MinThumbPX++
	value.Scrollbar.TrackColor = "#050505"
	value.Scrollbar.ThumbColor = "#060606"
	value.Scrollbar.ThumbHoverColor = "#070707"
	value.Scrollbar.ThumbPressColor = "#080808"
	value.Scrollbar.AutoHideDelayMS++
	value.Scrollbar.FadeMS++
	value.Scrollbar.PageStep += 0.01
	value.Scrollbar.TrackClick = "jump"
	value.Cursor.Shape = "block"
	value.Cursor.Blink = !value.Cursor.Blink
	value.Cursor.BlinkIntervalMS++
	value.Cursor.Thickness += 0.01
	value.Clipboard.OSC52 = "write"
	value.Render.Bidi = !value.Render.Bidi
	value.Render.TextGamma += 0.01
	value.Render.TextDarken += 0.01
	value.Render.TextRaster = "subpixel"
	value.Render.StatsHotkey += "+x"
	value.Render.ZoomInHotkey += "+x"
	value.Render.ZoomOutHotkey += "+x"
	value.Render.ZoomResetHotkey += "+x"
	value.Render.VSync = !value.Render.VSync
	value.Render.Redraw = "continuous"
	value.Render.Damage = "full"
	value.Shell.Program = "different-shell"
	value.Shell.Args = []string{"--different"}
	value.Shell.WorkingDirectory = "different-directory"
	value.Shell.Env = map[string]string{"SECRET_TOKEN": "must-not-appear"}
	return value
}

func TestDiffConfigCoversEveryConfigLeafInSchemaOrder(t *testing.T) {
	base := Defaults()
	changes := DiffConfig(fullyDifferentConfig(base), base)
	fields, err := SchemaFields(CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	expected := make([]ConfigChange, 0, len(changes))
	for _, field := range fields {
		if !field.Available || field.ApplyScope == "" || field.Path == "keys" || field.Path == "events" {
			continue
		}
		expected = append(expected, ConfigChange{Path: field.Path, Scope: field.ApplyScope})
	}
	if !reflect.DeepEqual(changes, expected) {
		t.Fatalf("changes mismatch\n got: %#v\nwant: %#v", changes, expected)
	}
	if len(changes) != 52 {
		t.Fatalf("config leaf count = %d, want 52", len(changes))
	}
}

func TestSchemaApplyCapabilitiesAreCompleteAndTruthful(t *testing.T) {
	fields, err := SchemaFields(CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]FieldMetadata, len(fields))
	for _, field := range fields {
		byPath[field.Path] = field
		if field.Available && field.Kind != KindTable && field.Path != "config_version" && field.ApplyScope == "" {
			t.Fatalf("available leaf %q has no apply scope", field.Path)
		}
		if field.Kind == KindTable && field.ApplyScope != "" {
			t.Fatalf("table %q exposed leaf apply scope %q", field.Path, field.ApplyScope)
		}
	}
	checks := map[string]ApplyScope{
		"window.opacity": ApplyLive, "colors.background": ApplyLive,
		"colors.foreground": ApplyLive, "cursor.shape": ApplyLive,
		"shell.program": ApplyNewPane, "window.width": ApplyNewWindow,
		"font.family": ApplyRestart, "render.vsync": ApplyRestart,
		"keys": ApplyLive, "events": ApplyLive,
	}
	for path, want := range checks {
		if got := byPath[path].ApplyScope; got != want {
			t.Fatalf("metadata %s scope = %q, want %q", path, got, want)
		}
	}
}

func TestPendingAndLiveMergeDoNotLeakOrApplyScopedValues(t *testing.T) {
	base := Defaults()
	desired := base.Clone()
	desired.Window.Opacity = 0.8
	desired.Cursor.Shape = "block"
	desired.Shell.Program = "future-shell"
	desired.Shell.Env = map[string]string{"SECRET_TOKEN": "must-not-appear"}
	desired.Font.Family = "Future Font"

	effective := MergeLiveConfig(base, desired)
	if effective.Window.Opacity != 0.8 || effective.Cursor.Shape != "block" {
		t.Fatalf("live values not merged: %#v", effective)
	}
	if effective.Shell.Program == desired.Shell.Program || effective.Font.Family == desired.Font.Family {
		t.Fatal("non-live values were merged into effective config")
	}
	pending := PendingConfigChanges(desired, effective)
	want := []ConfigChange{{Path: "font.family", Scope: ApplyRestart}, {Path: "shell.program", Scope: ApplyNewPane}, {Path: "shell.env", Scope: ApplyNewPane}}
	if !reflect.DeepEqual(pending, want) {
		t.Fatalf("pending = %#v, want %#v", pending, want)
	}
	for _, change := range pending {
		if change.Path == "SECRET_TOKEN" || change.Path == "must-not-appear" {
			t.Fatalf("sensitive value leaked: %#v", change)
		}
	}
}

func TestConfigCloneAndLiveMergeDetachMutableShellValues(t *testing.T) {
	base := Defaults()
	base.Shell.Args = []string{"base"}
	base.Shell.Env = map[string]string{"A": "base"}
	clone := base.Clone()
	clone.Shell.Args[0] = "mutated"
	clone.Shell.Env["A"] = "mutated"
	if base.Shell.Args[0] != "base" || base.Shell.Env["A"] != "base" {
		t.Fatal("Config.Clone leaked mutable shell state")
	}
	merged := MergeLiveConfig(base, fullyDifferentConfig(base))
	merged.Shell.Args[0] = "merged-mutation"
	merged.Shell.Env["A"] = "merged-mutation"
	if base.Shell.Args[0] != "base" || base.Shell.Env["A"] != "base" {
		t.Fatal("MergeLiveConfig leaked mutable base shell state")
	}
}
