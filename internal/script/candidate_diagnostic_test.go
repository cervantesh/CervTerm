package script

import (
	"path/filepath"
	"testing"

	"cervterm/internal/config"
)

func TestCandidateBundleExposesDetachedSourceGraphDiagnostic(t *testing.T) {
	dir := t.TempDir()
	writeSourceGraphScript(t, dir, "child.lua", `return {config_version=2,font={family="Child"}}`)
	primary := writeSourceGraphScript(t, dir, "primary.lua", `return {config_version=2,includes={"child.lua"}}`)
	bundle, err := BuildCandidateBundle(primary, config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()

	diagnostic := bundle.SourceGraphDiagnostic()
	if diagnostic.Primary == "" || len(diagnostic.Sources) != 2 || len(diagnostic.Edges) != 1 {
		t.Fatalf("source graph diagnostic = %#v", diagnostic)
	}
	if diagnostic.Sources[1].AuthoredVersion != 2 || diagnostic.Sources[1].EffectiveVersion != 2 {
		t.Fatalf("primary versions = %#v", diagnostic.Sources[1])
	}
	if got := filepath.Base(diagnostic.Edges[0].Requested); got != "child.lua" {
		t.Fatalf("edge requested path = %q", diagnostic.Edges[0].Requested)
	}

	diagnostic.Sources[0].SelectedPaths[0] = "mutated"
	diagnostic.Edges[0].From = "mutated"
	fresh := bundle.GraphDiagnostic()
	if fresh.Sources[0].SelectedPaths[0] == "mutated" || fresh.Edges[0].From == "mutated" {
		t.Fatalf("candidate diagnostic accessor leaked slice mutation: %#v", fresh)
	}
}
