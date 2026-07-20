package layoutstate

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func sampleDocument() Document {
	bg, text, blur, size := .8, .9, true, 13.0
	return Document{Version: 1, ActiveWorkspace: 0, Workspaces: []Workspace{{Name: "main", ActiveWindow: 0, Windows: []Window{{Title: "Terminal", Bounds: Bounds{X: 10, Y: 20, Width: 900, Height: 600, MonitorHint: "primary"}, ActiveTab: 0, Appearance: Appearance{ColorScheme: "dark", BackgroundOpacity: &bg, TextOpacity: &text, Blur: &blur, FontSize: &size}, Tabs: []Tab{{Title: "shell", FocusedLeaf: 0, Root: Node{Type: "pane", Launch: &Launch{TargetID: "local", Program: "bash", Args: []string{"-l"}, CWD: "/tmp"}}}}}}}}}
}

func TestGoldenAndRoundTrip(t *testing.T) {
	p, err := NewPlan(sampleDocument())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/layout-v1.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.TrimSpace(want)
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch\ngot %s\nwant %s", got, want)
	}
	decoded, err := Unmarshal(got)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, again) {
		t.Fatal("re-encode changed bytes")
	}
}

func TestInlineCanonicalV1(t *testing.T) {
	document := Document{Version: Version1, ActiveWorkspace: 0, Workspaces: []Workspace{{Name: "default", ActiveWindow: 0, Windows: []Window{oneWindow(paneNode())}}}}
	plan, err := NewPlan(document)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"active_workspace":0,"workspaces":[{"name":"default","active_window":0,"windows":[{"title":"","bounds":{"x":0,"y":0,"width":800,"height":600,"monitor_hint":""},"active_tab":0,"tabs":[{"title":"","focused_leaf":0,"root":{"type":"pane","launch":{"target_id":"","program":"","args":[],"cwd":""}}}],"appearance":{"color_scheme":""}}]}]}`
	for i := 0; i < 3; i++ {
		encoded, err := Marshal(plan)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != want {
			t.Fatalf("encoded=%s\nwant=%s", encoded, want)
		}
	}
}

func TestPlanDeepCopy(t *testing.T) {
	d := sampleDocument()
	p, err := NewPlan(d)
	if err != nil {
		t.Fatal(err)
	}
	d.Workspaces[0].Name = "changed"
	d.Workspaces[0].Windows[0].Tabs[0].Root.Launch.Args[0] = "changed"
	s := p.Snapshot()
	if s.Workspaces[0].Name != "main" || s.Workspaces[0].Windows[0].Tabs[0].Root.Launch.Args[0] != "-l" {
		t.Fatal("construction alias")
	}
	s.Workspaces[0].Windows[0].Tabs[0].Root.Launch.Program = "changed"
	if p.Snapshot().Workspaces[0].Windows[0].Tabs[0].Root.Launch.Program != "bash" {
		t.Fatal("snapshot alias")
	}
}

func TestCorruptionAndPrivacy(t *testing.T) {
	p, _ := NewPlan(sampleDocument())
	good, _ := Marshal(p)
	cases := []string{"", `{`, `[]`, `{}`, strings.Replace(string(good), `"version":1`, `"version":2`, 1), string(good) + ` {}`, strings.Replace(string(good), `"title":"shell"`, `"title":"shell","title":"again"`, 1), strings.Replace(string(good), `"cwd":"/tmp"`, `"cwd":"/tmp","env":"SECRET_SENTINEL"`, 1)}
	for _, in := range cases {
		if _, err := Unmarshal([]byte(in)); err == nil {
			t.Errorf("accepted %q", in)
		} else if strings.Contains(err.Error(), "SECRET_SENTINEL") {
			t.Fatal("error leaked value")
		}
	}
}

func TestPracticalLimits(t *testing.T) {
	d := sampleDocument()
	d.Workspaces[0].Name = strings.Repeat("a", MaxNameBytes)
	if _, err := NewPlan(d); err != nil {
		t.Fatal(err)
	}
	d.Workspaces[0].Name += `a`
	if _, err := NewPlan(d); err == nil {
		t.Fatal("accepted overlong name")
	}
	d = sampleDocument()
	d.Workspaces[0].Windows[0].Tabs[0].Root.Launch.Args = make([]string, MaxArgs)
	if _, err := NewPlan(d); err != nil {
		t.Fatal(err)
	}
	d.Workspaces[0].Windows[0].Tabs[0].Root.Launch.Args = append(d.Workspaces[0].Windows[0].Tabs[0].Root.Launch.Args, "x")
	if _, err := NewPlan(d); err == nil {
		t.Fatal("accepted too many args")
	}
}

func FuzzUnmarshalLayout(f *testing.F) {
	p, _ := NewPlan(sampleDocument())
	b, _ := Marshal(p)
	f.Add(b)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > MaxJSONBytes {
			return
		}
		p, err := Unmarshal(data)
		if err != nil {
			return
		}
		canonical, err := Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		p2, err := Unmarshal(canonical)
		if err != nil {
			t.Fatal(err)
		}
		canonical2, _ := Marshal(p2)
		if !bytes.Equal(canonical, canonical2) {
			t.Fatal("non-canonical roundtrip")
		}
	})
}
