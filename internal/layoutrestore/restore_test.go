package layoutrestore

import (
	"reflect"
	"strings"
	"testing"

	"cervterm/internal/layoutstate"
	"cervterm/internal/windowbounds"
)

func testOptions() Options {
	return Options{
		DefaultLaunch: Launch{Program: "shell", Args: []string{"-l"}, CWD: "/default"},
		Targets:       []Target{{ID: "dev", Program: "target", Args: []string{"one", "two"}, CWD: "/target"}},
		Monitors:      []windowbounds.Monitor{{ID: "m1", WorkArea: windowbounds.Rect{Width: 1920, Height: 1080}, ScaleX: 1.5, ScaleY: 1.5, Primary: true}},
		Policy:        windowbounds.Policy{FallbackWidth: 800, FallbackHeight: 600, MinWidth: 320, MinHeight: 200, ChromeHeight: 30, MinVisibleChromeX: 64, MinVisibleChromeY: 20},
		Appearance:    BlueprintAppearance{ColorScheme: "current", BackgroundOpacity: 1, TextOpacity: .9, FontSize: 14},
		CWDUsable:     func(cwd string) bool { return cwd != "/bad" },
	}
}

func testPlan(t *testing.T, launches ...layoutstate.Launch) layoutstate.Plan {
	t.Helper()
	if len(launches) == 0 {
		launches = []layoutstate.Launch{{}}
	}
	nodes := make([]layoutstate.Node, len(launches))
	for i := range launches {
		launch := launches[i]
		nodes[i] = layoutstate.Node{Type: "pane", Launch: &launch}
	}
	root := nodes[0]
	for i := 1; i < len(nodes); i++ {
		first, second := root, nodes[i]
		root = layoutstate.Node{Type: "split", Axis: "columns", Ratio: 5000, First: &first, Second: &second}
	}
	plan, err := layoutstate.NewPlan(layoutstate.Document{Version: 1, Workspaces: []layoutstate.Workspace{{Name: "ws", ActiveWindow: 0, Windows: []layoutstate.Window{{Title: "win", Bounds: layoutstate.Bounds{X: 10, Y: 20, Width: 900, Height: 700}, ActiveTab: 0, Tabs: []layoutstate.Tab{{Title: "tab", FocusedLeaf: len(nodes) - 1, Root: root}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func firstLaunch(snapshot Snapshot) ResolvedLaunch {
	return *snapshot.Workspaces[0].Windows[0].Tabs[0].Root.Launch
}

func TestLaunchResolution(t *testing.T) {
	tests := []struct {
		name  string
		saved layoutstate.Launch
		want  ResolvedLaunch
	}{
		{"target", layoutstate.Launch{TargetID: "dev", Program: "old", Args: []string{"old"}, CWD: "/saved"}, ResolvedLaunch{"target", []string{"one", "two"}, "/saved", SourceCurrentTarget}},
		{"missing fallback", layoutstate.Launch{TargetID: "gone", Program: "fallback", Args: []string{"x"}, CWD: "/saved"}, ResolvedLaunch{"fallback", []string{"x"}, "/saved", SourcePersistedFallback}},
		{"missing default", layoutstate.Launch{TargetID: "gone", CWD: "/saved"}, ResolvedLaunch{"shell", []string{"-l"}, "/saved", SourceDefaultShell}},
		{"empty default", layoutstate.Launch{}, ResolvedLaunch{"shell", []string{"-l"}, "/default", SourceDefaultShell}},
		{"bad target cwd", layoutstate.Launch{TargetID: "dev", CWD: "/bad"}, ResolvedLaunch{"target", []string{"one", "two"}, "/target", SourceCurrentTarget}},
		{"bad persisted cwd", layoutstate.Launch{Program: "fallback", CWD: "/bad"}, ResolvedLaunch{"fallback", nil, "/default", SourcePersistedFallback}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blueprint, err := Prepare(testPlan(t, tt.saved), testOptions())
			if err != nil {
				t.Fatal(err)
			}
			if got := firstLaunch(blueprint.Snapshot()); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestValidationAndAllOrNothing(t *testing.T) {
	options := testOptions()
	options.Targets = append(options.Targets, options.Targets[0])
	if _, err := Prepare(testPlan(t), options); err == nil || !strings.Contains(err.Error(), "options.targets[1].id") {
		t.Fatalf("unexpected error: %v", err)
	}
	options = testOptions()
	options.DefaultLaunch.Program = ""
	_, err := Prepare(testPlan(t, layoutstate.Launch{Program: "ok"}, layoutstate.Launch{}), options)
	if err == nil || !strings.Contains(err.Error(), "tabs[0].root.second.launch: program") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBoundsAppearanceStructureAndCloning(t *testing.T) {
	bg, blur, size := .4, true, 18.0
	plan := testPlan(t, layoutstate.Launch{TargetID: "dev"}, layoutstate.Launch{Program: "fallback", Args: []string{"a"}})
	doc := plan.Snapshot()
	doc.Workspaces[0].Windows[0].Bounds = layoutstate.Bounds{X: 99999, Y: 99999, Width: 900, Height: 700}
	doc.Workspaces[0].Windows[0].Appearance = layoutstate.Appearance{ColorScheme: "saved", BackgroundOpacity: &bg, Blur: &blur, FontSize: &size}
	plan, _ = layoutstate.NewPlan(doc)
	options := testOptions()
	blueprint, err := Prepare(plan, options)
	if err != nil {
		t.Fatal(err)
	}
	options.DefaultLaunch.Args[0] = "mutated"
	options.Targets[0].Args[0] = "mutated"
	options.Monitors[0].ScaleX = 9
	first := blueprint.Snapshot()
	window := first.Workspaces[0].Windows[0]
	if window.Bounds.Outcome != windowbounds.FallbackOffscreen || window.Bounds.ScaleX != 1.5 {
		t.Fatalf("bounds: %#v", window.Bounds)
	}
	if window.Appearance != (BlueprintAppearance{ColorScheme: "saved", BackgroundOpacity: .4, TextOpacity: .9, Blur: true, FontSize: 18}) {
		t.Fatalf("appearance: %#v", window.Appearance)
	}
	if window.Tabs[0].FocusedLeaf != 1 || window.Tabs[0].Root.Axis != "columns" || window.Tabs[0].Root.Ratio != 5000 {
		t.Fatalf("structure lost: %#v", window.Tabs[0])
	}
	first.Workspaces[0].Windows[0].Tabs[0].Root.First.Launch.Args[0] = "snapshot mutation"
	second := blueprint.Snapshot()
	if second.Workspaces[0].Windows[0].Tabs[0].Root.First.Launch.Args[0] != "one" {
		t.Fatal("blueprint was mutated")
	}
	third, err := Prepare(plan, testOptions())
	if err != nil || !reflect.DeepEqual(second, third.Snapshot()) {
		t.Fatal("preparation is not deterministic")
	}
}

func TestPublicLaunchTypesHaveNoEnv(t *testing.T) {
	for _, value := range []any{Launch{}, Target{}, ResolvedLaunch{}} {
		typeOf := reflect.TypeOf(value)
		if _, ok := typeOf.FieldByName("Env"); ok {
			t.Fatalf("%s exposes Env", typeOf.Name())
		}
	}
}
