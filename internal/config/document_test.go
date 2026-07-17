package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

type v1CompatSnapshot struct {
	WindowWidth      int               `json:"window_width"`
	WindowHeight     int               `json:"window_height"`
	PaddingX         int               `json:"padding_x"`
	Scrollback       int               `json:"scrollback_history"`
	WheelMultiplier  int               `json:"wheel_multiplier"`
	ShellArgs        []string          `json:"shell_args"`
	ShellEnvironment map[string]string `json:"shell_env"`
}

func TestV1CompatibilityGolden(t *testing.T) {
	cfg, err := LoadLua(filepath.Join("testdata", "v1-compat", "permissive.lua"), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := v1CompatSnapshot{
		WindowWidth: cfg.Window.Width, WindowHeight: cfg.Window.Height, PaddingX: cfg.Window.PaddingX,
		Scrollback: cfg.Scrolling.History, WheelMultiplier: cfg.Scrolling.WheelMultiplier,
		ShellArgs: cfg.Shell.Args, ShellEnvironment: cfg.Shell.Env,
	}
	got, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	want, err := os.ReadFile(filepath.Join("testdata", "v1-compat", "permissive.want.json"))
	if err != nil {
		t.Fatal(err)
	}
	want = []byte(strings.ReplaceAll(string(want), "\r\n", "\n"))
	if string(got) != string(want) {
		t.Fatalf("v1 compatibility changed\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestV1ToV2AllFieldsSemanticGolden(t *testing.T) {
	base := Defaults()
	v1, err := LoadLua(filepath.Join("testdata", "v1-compat", "all-fields-v1.lua"), base)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := LoadLua(filepath.Join("testdata", "v1-compat", "all-fields-v2.lua"), base)
	if err != nil {
		t.Fatal(err)
	}
	if err := v1.Validate(); err != nil {
		t.Fatalf("v1 fixture: %v", err)
	}
	if err := v2.Validate(); err != nil {
		t.Fatalf("v2 fixture: %v", err)
	}
	if !reflect.DeepEqual(v1, v2) {
		t.Fatalf("v1/v2 semantic mismatch\nv1: %#v\nv2: %#v", v1, v2)
	}
	got, err := json.MarshalIndent(v1, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	want, err := os.ReadFile(filepath.Join("testdata", "v1-compat", "all-fields.want.json"))
	if err != nil {
		t.Fatal(err)
	}
	want = []byte(strings.ReplaceAll(string(want), "\r\n", "\n"))
	if string(got) != string(want) {
		t.Fatalf("all-fields semantic golden changed\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestConfigVersionDispatch(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr string
	}{
		{name: "omitted", version: ""},
		{name: "explicit v1", version: "config_version = 1,"},
		{name: "explicit v2", version: "config_version = 2,"},
		{name: "string", version: `config_version = "2",`, wantErr: "must be an integer"},
		{name: "boolean", version: `config_version = true,`, wantErr: "must be an integer"},
		{name: "fraction", version: "config_version = 1.5,", wantErr: "finite integer"},
		{name: "zero", version: "config_version = 0,", wantErr: "older than"},
		{name: "negative", version: "config_version = -1,", wantErr: "older than"},
		{name: "future", version: "config_version = 3,", wantErr: "requires a newer CervTerm"},
		{name: "infinity", version: "config_version = (1/0),", wantErr: "finite integer"},
		{name: "nan", version: "config_version = (0/0),", wantErr: "finite integer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeLuaDocument(t, "return { "+tt.version+" }")
			_, err := LoadLua(path, Defaults())
			if tt.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "config_version")) {
				t.Fatalf("error = %v, want config_version and %q", err, tt.wantErr)
			}
		})
	}
}

func TestV2StrictStructuralValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "valid", body: `window = { width = 1200 }, shell = { args = {"pwsh"}, env = {A = "B"} }`},
		{name: "unknown root", body: `windwo = {}`, wantErr: "windwo: unknown field"},
		{name: "non-string root key", body: `[{}] = true`, wantErr: "root: field names must be strings, got table key"},
		{name: "non-string nested key", body: `window = {[function() end] = true}`, wantErr: "window: field names must be strings, got function key"},
		{name: "unknown nested", body: `window = { widht = 100 }`, wantErr: "window.widht: unknown field"},
		{name: "wrong section", body: `window = "wide"`, wantErr: "window: must be table"},
		{name: "wrong scalar", body: `window = { width = "wide" }`, wantErr: "window.width: must be integer"},
		{name: "fractional integer", body: `scrolling = { history = 4.2 }`, wantErr: "scrolling.history: must be an integer"},
		{name: "infinite number", body: `font = { size = (1/0) }`, wantErr: "font.size: must be finite"},
		{name: "sparse args", body: `shell = { args = {[2] = "pwsh"} }`, wantErr: "dense 1-based array"},
		{name: "nondeterministic args key", body: `shell = { args = {[function() end] = "pwsh"} }`, wantErr: `invalid key "type function"`},
		{name: "bad args entry", body: `shell = { args = {"pwsh", 3} }`, wantErr: "shell.args[2]: must be string"},
		{name: "bad env value", body: `shell = { env = {TOKEN = 4} }`, wantErr: "shell.env.TOKEN: must be string"},
		{name: "unknown key field", body: `keys = {{key = "a", action = function() end, repeatable = true}}`, wantErr: "keys[1].repeatable: unknown field"},
		{name: "missing action", body: `keys = {{key = "a"}}`, wantErr: "keys[1].action"},
		{name: "foreign action userdata", body: `keys = {{key = "a", action = io.stdout}}`, wantErr: "userdata is not a cervterm action"},
		{name: "unknown event", body: `events = {ready = function() end}`, wantErr: "events.ready: unknown field"},
		{name: "bad event", body: `events = {bell = true}`, wantErr: "events.bell: must be a function"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeLuaDocument(t, "return { config_version = 2, "+tt.body+" }")
			_, err := LoadLua(path, Defaults())
			if tt.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCompositionFieldsAreVersionedAndUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "v1", want: "requires config_version = 2"},
		{name: "v2", version: "config_version = 2,", want: "later Phase 2 slice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeLuaDocument(t, `return { `+tt.version+` includes = {"base.lua"} }`)
			_, err := LoadLua(path, Defaults())
			if err == nil || !strings.Contains(err.Error(), "includes") || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want includes and %q", err, tt.want)
			}
		})
	}
}

func TestDocumentTracksPresenceAndMigration(t *testing.T) {
	state := lua.NewState()
	defer state.Close()
	if err := state.DoString(`return { window = { width = 1200, height = "legacy wrong type" }, shell = { args = {} } }`); err != nil {
		t.Fatal(err)
	}
	root := state.Get(-1).(*lua.LTable)
	document, err := DecodeDocument("presence.lua", root)
	if err != nil {
		t.Fatal(err)
	}
	if document.AuthoredVersion != 1 || document.Version != 2 || !reflect.DeepEqual(document.Migrations, []MigrationStep{{From: 1, To: 2}}) {
		t.Fatalf("document versions = authored %d effective %d migrations %#v", document.AuthoredVersion, document.Version, document.Migrations)
	}
	for _, path := range []string{"window", "window.width", "window.height", "shell", "shell.args"} {
		if !document.Has(path) {
			t.Fatalf("missing presence for %q", path)
		}
	}
	if document.Has("window.padding_x") || document.Has("font") {
		t.Fatalf("unexpected presence: %#v", document.Present)
	}
}

func TestSchemaMetadataIsDeterministic(t *testing.T) {
	first, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("schema metadata order is not deterministic")
	}
	if len(first) == 0 || first[0] != (FieldMetadata{Path: "config_version", Kind: KindInteger, Available: true}) {
		t.Fatalf("version metadata = %#v", first)
	}
	find := func(path string) (FieldMetadata, bool) {
		for _, field := range first {
			if field.Path == path {
				return field, true
			}
		}
		return FieldMetadata{}, false
	}
	if field, ok := find("window.width"); !ok || field.Kind != KindInteger || !field.Available {
		t.Fatalf("window.width metadata = %#v, %v", field, ok)
	}
	if field, ok := find("includes"); !ok || field.Kind != KindStringList || field.Available {
		t.Fatalf("includes metadata = %#v, %v", field, ok)
	}
}

func TestLoadLuaEvaluatesSourceOnce(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.ToSlash(filepath.Join(dir, "marker.txt"))
	path := filepath.Join(dir, "cervterm.lua")
	source := `local f = assert(io.open("` + marker + `", "a")); f:write("x"); f:close(); return { config_version = 2 }`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLua(path, Defaults()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "marker.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "x" {
		t.Fatalf("source evaluated %d times, marker=%q", len(got), got)
	}
}

func writeLuaDocument(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
