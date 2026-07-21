package config

import (
	"reflect"
	"testing"

	"cervterm/internal/fontdesc"
	"cervterm/internal/quickselect"
)

func fullyDifferentConfig(base Config) Config {
	value := base.Clone()
	value.Window.Width++
	value.Window.Height++
	value.Window.InitialRows = 24
	value.Window.InitialCols = 80
	value.Window.Decorations = "none"
	value.Window.Titlebar = "system"
	value.Window.PaddingX++
	value.Window.PaddingY++
	value.Window.PaddingLeft++
	value.Window.PaddingRight++
	value.Window.PaddingTop++
	value.Window.PaddingBottom++
	value.Window.DynamicTitle = !value.Window.DynamicTitle
	value.Window.Opacity = 0.75
	value.Window.TextOpacity = 0.7
	value.Window.BackgroundOpacity = 0.8
	value.Window.Blur = !value.Window.Blur
	value.LayoutPersistence.Enabled = !value.LayoutPersistence.Enabled
	value.LayoutPersistence.Path = "different-layout.json"
	value.Font.Family += " Different"
	value.Font.Descriptors = []fontdesc.Descriptor{{Family: "Different Font", Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100, AttributeMode: fontdesc.AttributeModeAugment}}
	value.Font.Fallback = []fontdesc.Descriptor{{Family: "Different Fallback"}}
	value.Font.Rules = []fontdesc.Rule{{Match: fontdesc.RuleMatch{Class: fontdesc.SymbolClassEmoji}, Use: fontdesc.Descriptor{Family: "Different Rule"}}}
	value.Font.Size++
	value.Font.Ligatures = !value.Font.Ligatures
	value.Font.Features = map[string]int{"ss01": 1}
	value.Font.LineHeight += 0.1
	value.Font.CellWidth += 0.1
	value.Font.BaselineOffset++
	value.Font.GlyphOffsetX++
	value.Font.GlyphOffsetY++
	value.ColorScheme = "Different"
	value.Colors.Foreground = "#010101"
	value.Colors.Background = "#020202"
	value.Colors.Cursor = "#030303"
	value.Colors.SelectionBackground = "#040404"
	value.Colors.ChromeBackground = "#11121314"
	value.Colors.ChromeMuted = "#21222324"
	value.Colors.Accent = "#31323334"
	value.Colors.Split = "#41424344"
	value.Colors.SearchMatch = "#51525354"
	value.Colors.Error = "#61626364"
	value.Colors.ANSI[0] = "#111111"
	_ = value.Colors.IndexedColors.Set(196, "#222222")
	value.Background.Layers = []BackgroundLayer{{Kind: "solid", Opacity: 1, Color: "#010203"}}
	value.Scrolling.History++
	value.Scrolling.WheelMultiplier++
	value.Scrolling.HideCursorWhenScrolled = !value.Scrolling.HideCursorWhenScrolled
	value.Scrollbar.Enabled = !value.Scrollbar.Enabled
	value.Scrollbar.Mode = "always"
	value.Scrollbar.StableGutter = !value.Scrollbar.StableGutter
	value.Scrollbar.AnimationFPS++
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
	value.TabBar.Mode = "always"
	value.TabBar.Position = "bottom"
	value.TabBar.HeightPX++
	value.TabBar.MinWidthPX++
	value.TabBar.MaxWidthPX++
	value.TabBar.PaddingX++
	value.TabBar.ShowNewButton = !value.TabBar.ShowNewButton
	value.TabBar.ShowCloseButton = !value.TabBar.ShowCloseButton
	value.Clipboard.OSC52 = "write"
	value.IME.Enabled = !value.IME.Enabled
	value.Accessibility.Enabled = !value.Accessibility.Enabled
	value.Accessibility.Scope = "different"
	value.Graphics.Kitty.Enabled = !value.Graphics.Kitty.Enabled
	value.Graphics.Limits.EncodedBytesPerPane--
	value.Graphics.Limits.DecodedBytesPerPane--
	value.Graphics.Limits.ImageCountPerPane--
	value.Graphics.Limits.PlacementCountPerPane--
	value.Graphics.Limits.GPUBytesPerContext--
	value.Bell.Mode = "visual"
	value.Bell.Focus = "always"
	value.Bell.ThrottleMS++
	value.Bell.VisualDurationMS++
	value.Notification.Enabled = true
	value.Notification.Focus = "always"
	value.Notification.RateLimitMS++
	value.Render.Bidi = !value.Render.Bidi
	value.Render.TextGamma += 0.01
	value.Render.TextDarken += 0.01
	value.Render.TextRaster = "subpixel"
	value.Render.StatsHotkey += "+x"
	value.Render.ZoomInHotkey += "+x"
	value.Render.ZoomOutHotkey += "+x"
	value.Render.ZoomResetHotkey += "+x"
	value.Render.VSync = !value.Render.VSync
	value.Render.MaxFPS = 60
	value.Render.Redraw = "continuous"
	value.Render.Damage = "full"
	value.Shell.Program = "different-shell"
	value.Shell.Args = []string{"--different"}
	value.Shell.WorkingDirectory = "different-directory"
	value.Shell.Env = map[string]string{"SECRET_TOKEN": "must-not-appear"}
	value.LaunchMenu = []LaunchTarget{{ID: "tool", Label: "Tool", Program: "tool"}}
	value.QuickSelect.Rules = []QuickSelectRule{{ID: "url", Pattern: "https://", Action: quickselect.ActionOpen}}
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
	if len(changes) != 112 {
		t.Fatalf("config leaf count = %d, want 112", len(changes))
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
		"window.text_opacity": ApplyLive, "window.background_opacity": ApplyLive,
		"colors.foreground": ApplyLive, "cursor.shape": ApplyLive,
		"shell.program": ApplyNewPane, "window.width": ApplyNewWindow,
		"font.family": ApplyRestart, "render.vsync": ApplyRestart,
		"window.padding_left": ApplyRestart, "window.padding_bottom": ApplyRestart,
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
	desired.Font.Fallback = []fontdesc.Descriptor{{Family: "Future Fallback"}}
	desired.Font.Rules = []fontdesc.Rule{{Match: fontdesc.RuleMatch{Class: fontdesc.SymbolClassEmoji}, Use: fontdesc.Descriptor{Family: "Future Rule"}}}

	effective := MergeLiveConfig(base, desired)
	if effective.Window.Opacity != 0.8 || effective.Cursor.Shape != "block" {
		t.Fatalf("live values not merged: %#v", effective)
	}
	if effective.Shell.Program == desired.Shell.Program || effective.Font.Family == desired.Font.Family || len(effective.Font.Fallback) != 0 || len(effective.Font.Rules) != 0 {
		t.Fatal("non-live values were merged into effective config")
	}
	pending := PendingConfigChanges(desired, effective)
	want := []ConfigChange{{Path: "font.family", Scope: ApplyRestart}, {Path: "font.fallback", Scope: ApplyRestart}, {Path: "font.rules", Scope: ApplyRestart}, {Path: "shell.program", Scope: ApplyNewPane}, {Path: "shell.env", Scope: ApplyNewPane}}
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
	clone.Font.Descriptors = []fontdesc.Descriptor{{Family: "clone"}}
	if base.Shell.Args[0] != "base" || base.Shell.Env["A"] != "base" || len(base.Font.Descriptors) != 0 {
		t.Fatal("Config.Clone leaked mutable state")
	}
	merged := MergeLiveConfig(base, fullyDifferentConfig(base))
	merged.Shell.Args[0] = "merged-mutation"
	merged.Shell.Env["A"] = "merged-mutation"
	if base.Shell.Args[0] != "base" || base.Shell.Env["A"] != "base" {
		t.Fatal("MergeLiveConfig leaked mutable base shell state")
	}
}
