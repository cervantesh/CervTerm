package config

import (
	"strconv"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func paddingDocument(t *testing.T, source string) Document {
	t.Helper()
	state := lua.NewState()
	t.Cleanup(state.Close)
	if err := state.DoString(source); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	root, ok := state.Get(-1).(*lua.LTable)
	if !ok {
		t.Fatalf("config returned %s, want table", state.Get(-1).Type())
	}
	document, err := DecodeDocument("padding-test.lua", root)
	if err != nil {
		t.Fatalf("DecodeDocument: %v", err)
	}
	return document
}

func TestWindowPaddingAliasesProjectToSides(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   WindowConfig
	}{
		{
			name:   "v1 aliases preserve effective geometry",
			source: `return { window = { padding_x = 10, padding_y = 11 } }`,
			want:   WindowConfig{PaddingX: 10, PaddingY: 11, PaddingLeft: 10, PaddingRight: 10, PaddingTop: 11, PaddingBottom: 11},
		},
		{
			name:   "v2 explicit sides override aliases",
			source: `return { config_version = 2, window = { padding_x = 10, padding_y = 11, padding_left = 1, padding_bottom = 4 } }`,
			want:   WindowConfig{PaddingX: 10, PaddingY: 11, PaddingLeft: 1, PaddingRight: 10, PaddingTop: 11, PaddingBottom: 4},
		},
		{
			name:   "v1 ignores v2 side fields",
			source: `return { window = { padding_x = 10, padding_y = 11, padding_left = 1, padding_bottom = 4 } }`,
			want:   WindowConfig{PaddingX: 10, PaddingY: 11, PaddingLeft: 10, PaddingRight: 10, PaddingTop: 11, PaddingBottom: 11},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := FromDocument(Defaults(), paddingDocument(t, test.source)).Window
			if got.PaddingX != test.want.PaddingX || got.PaddingY != test.want.PaddingY ||
				got.PaddingLeft != test.want.PaddingLeft || got.PaddingRight != test.want.PaddingRight ||
				got.PaddingTop != test.want.PaddingTop || got.PaddingBottom != test.want.PaddingBottom {
				t.Fatalf("padding = %#v, want aliases/sides %#v", got, test.want)
			}
		})
	}
}

func TestWindowSidePaddingStrictBounds(t *testing.T) {
	for _, value := range []int{-1, MaxWindowPadding + 1} {
		state := lua.NewState()
		if err := state.DoString(`return { config_version = 2, window = { padding_left = ` + strconv.Itoa(value) + ` } }`); err != nil {
			state.Close()
			t.Fatal(err)
		}
		_, err := DecodeDocument("padding-test.lua", state.Get(-1).(*lua.LTable))
		state.Close()
		if err == nil || !strings.Contains(err.Error(), "window.padding_left") {
			t.Fatalf("padding_left=%d error = %v, want path-specific bounds error", value, err)
		}
	}
}

func TestWindowSidePaddingSchemaIsV2RestartOnly(t *testing.T) {
	v1, err := SchemaFields(1)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range v1 {
		if strings.HasPrefix(field.Path, "window.padding_") && field.Path != "window.padding_x" && field.Path != "window.padding_y" {
			t.Fatalf("v1 unexpectedly exposes %q", field.Path)
		}
	}
	for _, path := range []string{"window.padding_left", "window.padding_right", "window.padding_top", "window.padding_bottom"} {
		found := false
		for _, field := range v2 {
			if field.Path == path {
				found = true
				if field.ApplyScope != ApplyRestart || field.RuntimeOverride {
					t.Fatalf("%s metadata = %#v", path, field)
				}
			}
		}
		if !found {
			t.Fatalf("v2 missing %s", path)
		}
	}
}
