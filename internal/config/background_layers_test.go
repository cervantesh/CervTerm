package config

import (
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestBackgroundLayersDecodeDefaultsAndV1Isolation(t *testing.T) {
	v2 := paddingDocument(t, `return { config_version=2, background={ layers={
		{kind="solid", color="#010203"},
		{kind="linear_gradient", colors={"#000000", "#ffffff80"}, angle=45, opacity=.5},
		{kind="image", path="wall.png"},
	} } }`)
	cfg := FromDocument(Defaults(), v2)
	if len(cfg.Background.Layers) != 3 {
		t.Fatalf("layers = %#v", cfg.Background.Layers)
	}
	image := cfg.Background.Layers[2]
	if image.Opacity != 1 || image.Fit != "cover" || image.HorizontalAlign != "center" || image.VerticalAlign != "center" {
		t.Fatalf("image defaults = %#v", image)
	}
	v1 := paddingDocument(t, `return { background={ layers={{kind="solid", color="#010203"}} } }`)
	if got := FromDocument(Defaults(), v1).Background.Layers; len(got) != 0 {
		t.Fatalf("v1 applied background layers: %#v", got)
	}
}

func TestBackgroundLayersStrictUnionDiagnostics(t *testing.T) {
	for _, test := range []struct{ name, layer, want string }{
		{"unknown", `{kind="solid", color="#010203", path="x"}`, "path"},
		{"missing color", `{kind="solid"}`, "color"},
		{"few gradient colors", `{kind="linear_gradient", colors={"#000000"}}`, "2..8"},
		{"empty image path", `{kind="image", path=""}`, "non-empty"},
		{"bad image fit", `{kind="image", path="x", fit="tile"}`, "unsupported"},
		{"bad opacity", `{kind="solid", color="#000000", opacity=2}`, "opacity"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := lua.NewState()
			t.Cleanup(state.Close)
			if err := state.DoString(`return {config_version=2, background={layers={` + test.layer + `}}}`); err != nil {
				t.Fatal(err)
			}
			_, err := DecodeDocument("background-test.lua", state.Get(-1).(*lua.LTable))
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "background.layers[1]") {
				t.Fatalf("error = %v, want indexed %q", err, test.want)
			}
		})
	}
}

func TestBackgroundLayersReplaceWithWinnerProvenance(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2, includes={"base.lua"}, background={layers={{kind="solid",color="#222222"}}}}`)
	writeGraphLua(t, dir, "base.lua", `return {config_version=2, background={layers={{kind="solid",color="#111111"},{kind="image",path="base.png"}}}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if len(cfg.Background.Layers) != 1 || cfg.Background.Layers[0].Color != "#222222" {
		t.Fatalf("replacement layers = %#v", cfg.Background.Layers)
	}
	record, ok := composition.Provenance.Lookup("background.layers")
	if !ok || record.Winner.CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "primary.lua")) {
		t.Fatalf("provenance = %#v ok=%v", record, ok)
	}
}

func TestBackgroundLayersCloneAndDiff(t *testing.T) {
	cfg := Defaults()
	cfg.Background.Layers = []BackgroundLayer{{Kind: "linear_gradient", Opacity: 1, Colors: []string{"#000000", "#ffffff"}}}
	clone := cfg.Clone()
	clone.Background.Layers[0].Colors[0] = "#112233"
	if cfg.Background.Layers[0].Colors[0] != "#000000" {
		t.Fatal("clone aliases colors")
	}
	changes := DiffConfig(clone, cfg)
	found := false
	for _, change := range changes {
		if change.Path == "background.layers" && change.Scope == ApplyLive {
			found = true
		}
	}
	if !found {
		t.Fatalf("changes = %#v", changes)
	}
}
