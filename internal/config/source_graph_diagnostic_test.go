package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSourceGraphDiagnosticIsDetachedAndExcludesExecutableState(t *testing.T) {
	graph := &SourceGraph{
		Primary: "canonical/primary.lua",
		Sources: []SourceNode{{
			RequestedPath: "primary.lua", CanonicalPath: "canonical/primary.lua",
			SelectedPath: "selected/primary.lua", SelectedPaths: []string{"selected/primary.lua", "alias/primary.lua"},
			Hash: [32]byte{1, 2, 3}, Document: Document{AuthoredVersion: 1, Version: 2},
		}},
		Edges: []SourceEdge{{From: "canonical/primary.lua", To: "canonical/child.lua", Requested: "child.lua"}},
		Dependencies: []SourceDependency{{
			Kind: DependencyRequire, Requested: "helpers", Canonical: "canonical/helpers.lua",
			Selected: "selected/helpers.lua", Hash: [32]byte{4, 5, 6},
		}},
		StagedTeal: []StagedTeal{{}},
	}

	diagnostic := graph.Diagnostic()
	if diagnostic.Primary != graph.Primary || diagnostic.Sources[0].AuthoredVersion != 1 || diagnostic.Sources[0].EffectiveVersion != 2 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"hash", "document", "staged_teal", "source_bytes", "evaluation_lua"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("diagnostic contains forbidden %q state: %s", forbidden, encoded)
		}
	}

	diagnostic.Sources[0].SelectedPaths[0] = "mutated"
	diagnostic.Sources = append(diagnostic.Sources, SourceNodeDiagnostic{CanonicalPath: "extra"})
	diagnostic.Edges[0].From = "mutated"
	diagnostic.Dependencies[0].Canonical = "mutated"
	fresh := graph.Diagnostic()
	if fresh.Sources[0].SelectedPaths[0] != "selected/primary.lua" || len(fresh.Sources) != 1 || fresh.Edges[0].From != "canonical/primary.lua" || fresh.Dependencies[0].Canonical != "canonical/helpers.lua" {
		t.Fatalf("graph diagnostic was not detached: %#v", fresh)
	}

	clone := fresh.Clone()
	clone.Sources[0].SelectedPaths[0] = "clone mutation"
	if fresh.Sources[0].SelectedPaths[0] != "selected/primary.lua" {
		t.Fatal("diagnostic clone leaked nested slice mutation")
	}
}
