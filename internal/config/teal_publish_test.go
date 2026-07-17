package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestPublishStagedTealCreatesOwnedOutput(t *testing.T) {
	dir := t.TempDir()
	installFakeGraphTeal(t, dir)
	source := writeGraphLua(t, dir, "primary.tl", `return {config_version=2}`)
	state, graph := buildTealPublishGraph(t, source)
	defer state.Close()
	defer graph.Close()
	result, err := PublishStagedTeal(graph, TealPublicationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) != 1 || result.Outputs[0].Adopted {
		t.Fatalf("publication result = %#v", result)
	}
	assertFileBytes(t, graph.StagedTeal[0].PublishedLua, []byte(`return {config_version=2}`))
	markerPath := TealOwnershipMarkerPath(graph.StagedTeal[0].PublishedLua)
	content, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	var marker tealOwnershipMarker
	if err := json.Unmarshal(content, &marker); err != nil || marker.Source != canonicalTestSource(t, source) {
		t.Fatalf("marker=%#v content=%q err=%v", marker, content, err)
	}
}

func TestPublishStagedTealAdoptsIdenticalLegacyOutput(t *testing.T) {
	dir := t.TempDir()
	installFakeGraphTeal(t, dir)
	body := `return {config_version=2,font={size=12}}`
	source := writeGraphLua(t, dir, "primary.tl", body)
	published := filepath.Join(dir, "primary.lua")
	if err := os.WriteFile(published, []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
	state, graph := buildTealPublishGraph(t, source)
	defer state.Close()
	defer graph.Close()
	result, err := PublishStagedTeal(graph, TealPublicationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) != 1 || !result.Outputs[0].Adopted {
		t.Fatalf("adoption result = %#v", result)
	}
	if _, err := os.Stat(TealOwnershipMarkerPath(published)); err != nil {
		t.Fatal(err)
	}
}

func TestPublishStagedTealRejectsUnownedOrForeignOutput(t *testing.T) {
	tests := []struct {
		name   string
		marker string
		want   string
	}{
		{name: "unowned differing", want: "refusing to overwrite unowned"},
		{name: "malformed marker", marker: `{bad`, want: "invalid or foreign ownership marker"},
		{name: "foreign marker", marker: `{"version":1,"source":"foreign.tl"}`, want: "invalid or foreign ownership marker"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			installFakeGraphTeal(t, dir)
			source := writeGraphLua(t, dir, "primary.tl", `return {config_version=2}`)
			published := filepath.Join(dir, "primary.lua")
			old := []byte("user-owned")
			if err := os.WriteFile(published, old, 0o600); err != nil {
				t.Fatal(err)
			}
			if test.marker != "" {
				if err := os.WriteFile(TealOwnershipMarkerPath(published), []byte(test.marker), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			state, graph := buildTealPublishGraph(t, source)
			defer state.Close()
			defer graph.Close()
			_, err := PublishStagedTeal(graph, TealPublicationOptions{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want %q", err, test.want)
			}
			assertFileBytes(t, published, old)
		})
	}
}

func TestPublishStagedTealReplacesOwnedOutput(t *testing.T) {
	dir := t.TempDir()
	installFakeGraphTeal(t, dir)
	source := writeGraphLua(t, dir, "primary.tl", `return {config_version=2,font={size=10}}`)
	state, graph := buildTealPublishGraph(t, source)
	if _, err := PublishStagedTeal(graph, TealPublicationOptions{}); err != nil {
		t.Fatal(err)
	}
	graph.Close()
	state.Close()
	writeGraphLua(t, dir, "primary.tl", `return {config_version=2,font={size=20}}`)
	state, graph = buildTealPublishGraph(t, source)
	defer state.Close()
	defer graph.Close()
	if _, err := PublishStagedTeal(graph, TealPublicationOptions{}); err != nil {
		t.Fatal(err)
	}
	assertFileBytes(t, filepath.Join(dir, "primary.lua"), []byte(`return {config_version=2,font={size=20}}`))
}

func TestPublishStagedTealRollsBackMultipleOutputs(t *testing.T) {
	dir := t.TempDir()
	installFakeGraphTeal(t, dir)
	primary := writeGraphLua(t, dir, "primary.tl", `return {config_version=2,includes={"child.tl"},font={size=10}}`)
	writeGraphLua(t, dir, "child.tl", `return {config_version=2,font={family="old"}}`)
	state, graph := buildTealPublishGraph(t, primary)
	if _, err := PublishStagedTeal(graph, TealPublicationOptions{}); err != nil {
		t.Fatal(err)
	}
	oldFiles := snapshotPublishedFiles(t, graph)
	graph.Close()
	state.Close()
	writeGraphLua(t, dir, "primary.tl", `return {config_version=2,includes={"child.tl"},font={size=20}}`)
	writeGraphLua(t, dir, "child.tl", `return {config_version=2,font={family="new"}}`)
	state, graph = buildTealPublishGraph(t, primary)
	defer state.Close()
	defer graph.Close()
	_, err := PublishStagedTeal(graph, TealPublicationOptions{FaultInjector: func(index int, step string) error {
		if index == 1 && step == "output" {
			return errors.New("boom")
		}
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "injected Teal publication failure") {
		t.Fatalf("fault error = %v", err)
	}
	for path, content := range oldFiles {
		assertFileBytes(t, path, content)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".cervterm-publish-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("publication temps = %#v err=%v", matches, err)
	}
}

func TestPublishStagedTealRejectsModuleCollision(t *testing.T) {
	dir := t.TempDir()
	installFakeGraphTeal(t, dir)
	generatedLua := filepath.Join(dir, "generated.lua")
	if err := os.WriteFile(generatedLua, []byte(`return {value=1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	writeGraphLua(t, dir, "generated.tl", `return {config_version=2}`)
	primary := writeGraphLua(t, dir, "primary.lua", `package.path=`+luaQuote(filepath.Join(dir, "?.lua"))+`; require("generated"); return {config_version=2,includes={"generated.tl"}}`)
	state, graph := buildTealPublishGraph(t, primary)
	defer state.Close()
	defer graph.Close()
	_, err := PublishStagedTeal(graph, TealPublicationOptions{})
	if err == nil || !strings.Contains(err.Error(), "collides with explicit require dependency") {
		t.Fatalf("dependency collision error = %v", err)
	}
	assertFileBytes(t, generatedLua, []byte(`return {value=1}`))
}

func TestPublishStagedTealRejectsChangedDuplicateAndNonregularPaths(t *testing.T) {
	t.Run("changed after prepare", func(t *testing.T) {
		dir := t.TempDir()
		installFakeGraphTeal(t, dir)
		source := writeGraphLua(t, dir, "primary.tl", `return {config_version=2}`)
		published := filepath.Join(dir, "primary.lua")
		if err := os.WriteFile(published, []byte(`return {config_version=2}`), 0o600); err != nil {
			t.Fatal(err)
		}
		state, graph := buildTealPublishGraph(t, source)
		defer state.Close()
		defer graph.Close()
		_, err := PublishStagedTeal(graph, TealPublicationOptions{BeforeCommit: func() error { return os.WriteFile(published, []byte("changed"), 0o600) }})
		if err == nil || !strings.Contains(err.Error(), "changed after preparation") {
			t.Fatalf("changed error = %v", err)
		}
		assertFileBytes(t, published, []byte("changed"))
	})
	t.Run("duplicate destination", func(t *testing.T) {
		dir := t.TempDir()
		installFakeGraphTeal(t, dir)
		source := writeGraphLua(t, dir, "primary.tl", `return {config_version=2}`)
		state, graph := buildTealPublishGraph(t, source)
		defer state.Close()
		defer graph.Close()
		graph.StagedTeal = append(graph.StagedTeal, graph.StagedTeal[0])
		if _, err := PublishStagedTeal(graph, TealPublicationOptions{}); err == nil || !strings.Contains(err.Error(), "duplicate Teal publication destination") {
			t.Fatalf("duplicate error = %v", err)
		}
	})
	t.Run("nonregular and hardlink", func(t *testing.T) {
		for _, kind := range []string{"directory", "hardlink"} {
			t.Run(kind, func(t *testing.T) {
				dir := t.TempDir()
				installFakeGraphTeal(t, dir)
				source := writeGraphLua(t, dir, "primary.tl", `return {config_version=2}`)
				published := filepath.Join(dir, "primary.lua")
				if kind == "directory" {
					if err := os.Mkdir(published, 0o700); err != nil {
						t.Fatal(err)
					}
				} else {
					base := filepath.Join(dir, "base.lua")
					if err := os.WriteFile(base, []byte(`return {config_version=2}`), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.Link(base, published); err != nil {
						t.Skipf("hardlinks unavailable: %v", err)
					}
				}
				state, graph := buildTealPublishGraph(t, source)
				defer state.Close()
				defer graph.Close()
				_, err := PublishStagedTeal(graph, TealPublicationOptions{})
				want := "not a regular file"
				if kind == "hardlink" {
					want = "multiple hard links"
				}
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("%s error = %v", kind, err)
				}
			})
		}
	})
}

func TestPublishStagedTealFaultBoundariesAndStaleTempRecovery(t *testing.T) {
	for _, step := range []string{"marker", "output"} {
		t.Run(step, func(t *testing.T) {
			dir := t.TempDir()
			installFakeGraphTeal(t, dir)
			source := writeGraphLua(t, dir, "primary.tl", `return {config_version=2}`)
			stale := filepath.Join(dir, ".cervterm-publish-stale")
			fresh := filepath.Join(dir, ".cervterm-publish-fresh")
			if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fresh, []byte("fresh"), 0o600); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-25 * time.Hour)
			if err := os.Chtimes(stale, old, old); err != nil {
				t.Fatal(err)
			}
			state, graph := buildTealPublishGraph(t, source)
			defer state.Close()
			defer graph.Close()
			_, err := PublishStagedTeal(graph, TealPublicationOptions{FaultInjector: func(_ int, got string) error {
				if got == step {
					return errors.New("boom")
				}
				return nil
			}})
			if err == nil {
				t.Fatal("expected injected failure")
			}
			if _, err := os.Stat(graph.StagedTeal[0].PublishedLua); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("output remains: %v", err)
			}
			if _, err := os.Stat(TealOwnershipMarkerPath(graph.StagedTeal[0].PublishedLua)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("marker remains: %v", err)
			}
			if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stale temp remains: %v", err)
			}
			assertFileBytes(t, fresh, []byte("fresh"))
		})
	}
	result, err := PublishStagedTeal(&SourceGraph{}, TealPublicationOptions{})
	if err != nil || len(result.Outputs) != 0 {
		t.Fatalf("empty graph result=%#v err=%v", result, err)
	}
}

func buildTealPublishGraph(t *testing.T, primary string) (*lua.LState, *SourceGraph) {
	t.Helper()
	state := lua.NewState()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		state.Close()
		t.Fatal(err)
	}
	return state, graph
}

func snapshotPublishedFiles(t *testing.T, graph *SourceGraph) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	for _, staged := range graph.StagedTeal {
		for _, path := range []string{staged.PublishedLua, TealOwnershipMarkerPath(staged.PublishedLua)} {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			out[path] = content
		}
	}
	return out
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
