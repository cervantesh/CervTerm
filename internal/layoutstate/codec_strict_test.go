package layoutstate

import (
	"reflect"
	"strings"
	"testing"
)

func canonicalSample(t *testing.T) string {
	t.Helper()
	plan, err := NewPlan(sampleDocument())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestStrictCorruptionMatrix(t *testing.T) {
	good := canonicalSample(t)
	cases := map[string]string{
		"truncated":               good[:len(good)-1],
		"missing root":            strings.Replace(good, `"active_workspace":0,`, ``, 1),
		"missing nested":          strings.Replace(good, `"focused_leaf":0,`, ``, 1),
		"unknown root":            strings.Replace(good, `"version":1`, `"version":1,"unknown":0`, 1),
		"unknown workspace":       strings.Replace(good, `"name":"main"`, `"name":"main","workspace_id":7`, 1),
		"unknown window":          strings.Replace(good, `"title":"Terminal"`, `"title":"Terminal","renderer":"secret"`, 1),
		"unknown bounds":          strings.Replace(good, `"monitor_hint":"primary"`, `"monitor_hint":"primary","native_handle":"secret"`, 1),
		"unknown appearance":      strings.Replace(good, `"color_scheme":"dark"`, `"color_scheme":"dark","callback":"secret"`, 1),
		"unknown tab":             strings.Replace(good, `"title":"shell"`, `"title":"shell","revision":9`, 1),
		"unknown node":            strings.Replace(good, `"type":"pane"`, `"type":"pane","pane_id":4`, 1),
		"unknown launch":          strings.Replace(good, `"cwd":"/tmp"`, `"cwd":"/tmp","env":{"TOKEN":"SECRET_SENTINEL"}`, 1),
		"unknown sensitive key":   strings.Replace(good, `"version":1`, `"version":1,"SECRET_SENTINEL":0`, 1),
		"duplicate sensitive key": strings.Replace(good, `"version":1`, `"version":1,"SECRET_SENTINEL":0,"SECRET_SENTINEL":1`, 1),
		"duplicate root":          strings.Replace(good, `"version":1`, `"version":1,"version":1`, 1),
		"duplicate nested":        strings.Replace(good, `"cwd":"/tmp"`, `"cwd":"/tmp","cwd":"again"`, 1),
		"future version":          strings.Replace(good, `"version":1`, `"version":2`, 1),
		"zero version":            strings.Replace(good, `"version":1`, `"version":0`, 1),
		"args null":               strings.Replace(good, `"args":["-l"]`, `"args":null`, 1),
		"args wrong member":       strings.Replace(good, `"args":["-l"]`, `"args":[1]`, 1),
		"required number null":    strings.Replace(good, `"active_workspace":0`, `"active_workspace":null`, 1),
		"required string null":    strings.Replace(good, `"title":"shell"`, `"title":null`, 1),
		"optional number null":    strings.Replace(good, `"background_opacity":0.8`, `"background_opacity":null`, 1),
		"optional object null":    strings.Replace(good, `"type":"pane"`, `"type":"pane","first":null`, 1),
		"launch string null":      strings.Replace(good, `"cwd":"/tmp"`, `"cwd":null`, 1),
		"wrong node union":        strings.Replace(good, `"type":"pane"`, `"type":"split"`, 1),
		"unknown node type":       strings.Replace(good, `"type":"pane"`, `"type":"remote"`, 1),
		"bad focus index":         strings.Replace(good, `"focused_leaf":0`, `"focused_leaf":1`, 1),
		"trailing value":          good + `{}`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Unmarshal([]byte(input))
			if err == nil {
				t.Fatal("corruption accepted")
			}
			if strings.Contains(err.Error(), "SECRET_SENTINEL") || strings.Contains(err.Error(), `"secret"`) {
				t.Fatalf("error leaked rejected value: %v", err)
			}
		})
	}
}

func TestNilArgsCanonicalizeToArray(t *testing.T) {
	document := sampleDocument()
	document.Workspaces[0].Windows[0].Tabs[0].Root.Launch.Args = nil
	plan, err := NewPlan(document)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"args":[]`) || strings.Contains(string(encoded), `"args":null`) {
		t.Fatalf("encoded=%s", encoded)
	}
	if plan.Snapshot().Workspaces[0].Windows[0].Tabs[0].Root.Launch.Args == nil {
		t.Fatal("snapshot retained noncanonical nil args")
	}
}

func TestPersistenceSchemaStructurallyExcludesLiveAndSecretFields(t *testing.T) {
	forbidden := map[string]struct{}{
		"env": {}, "environment": {}, "process_id": {}, "pty": {}, "session": {}, "scrollback": {},
		"parser": {}, "renderer": {}, "callback": {}, "window_id": {}, "workspace_id": {}, "tab_id": {},
		"pane_id": {}, "split_id": {}, "revision": {}, "native_handle": {}, "gl_context": {},
	}
	types := []reflect.Type{
		reflect.TypeOf(Document{}), reflect.TypeOf(Workspace{}), reflect.TypeOf(Window{}), reflect.TypeOf(Bounds{}),
		reflect.TypeOf(Appearance{}), reflect.TypeOf(Tab{}), reflect.TypeOf(Node{}), reflect.TypeOf(Launch{}),
	}
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if _, blocked := forbidden[name]; blocked {
				t.Fatalf("%s exposes forbidden field %q", typ, name)
			}
		}
	}
	encoded := canonicalSample(t)
	for field := range forbidden {
		if strings.Contains(encoded, `"`+field+`"`) {
			t.Fatalf("encoded schema contains forbidden field %q", field)
		}
	}
}
