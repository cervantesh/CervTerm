package script

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/config"
)

func TestExplicitV1RetainsFailFastScriptSurfaces(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "malformed key",
			source: `return { config_version = 1, keys = { { key = "a" } } }`,
			want:   "action must be a function or cervterm action",
		},
		{
			name:   "malformed event",
			source: `return { config_version = 1, events = { bell = true } }`,
			want:   "events.bell must be a function",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeScriptConfig(t, tt.source)
			_, runtime, err := Load(path, config.Defaults())
			if runtime != nil {
				runtime.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestV2TypedScriptDocumentLoads(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
return {
  config_version = 2,
  keys = {
    { key = "c", label = "Copy", action = cervterm.action.CopySelection },
    { key = "n", action = function(term) term:notify("callback") end },
  },
  events = { bell = function(term) term:notify("bell") end },
}`)
	cfg, runtime, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	bindings := runtime.Bindings()
	if len(bindings) != 2 || bindings[0].Label != "Copy" {
		t.Fatalf("bindings = %#v", bindings)
	}
	if _, ok := bindings[0].Action.Action.(termaction.CopySelection); !ok {
		t.Fatalf("typed action = %#v", bindings[0].Action)
	}
	host := &fakeHost{}
	if err := runtime.Dispatch(1, host); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(host.notices, ""); got != "callback" {
		t.Fatalf("callback notice = %q", got)
	}
}

func TestV2ScriptUnknownBindingAndEventFieldsFailEarly(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "binding field",
			source: `return { config_version = 2, keys = {{key = "a", action = function() end, typo = true}} }`,
			want:   "keys[1].typo: unknown field",
		},
		{
			name:   "event field",
			source: `return { config_version = 2, events = { ready = function() end } }`,
			want:   "events.ready: unknown field",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeScriptConfig(t, tt.source)
			_, runtime, err := Load(path, config.Defaults())
			if runtime != nil {
				runtime.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestScriptLoadEvaluatesV2SourceOnce(t *testing.T) {
	dir := t.TempDir()
	markerPath := filepath.Join(dir, "marker.txt")
	markerLua := filepath.ToSlash(markerPath)
	configPath := filepath.Join(dir, "cervterm.lua")
	source := `local f = assert(io.open("` + markerLua + `", "a")); f:write("x"); f:close(); return { config_version = 2 }`
	if err := os.WriteFile(configPath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	_, runtime, err := Load(configPath, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runtime.Close()
	got, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "x" {
		t.Fatalf("source evaluated %d times, marker=%q", len(got), got)
	}
}
