package script

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/config"
)

func TestCandidateBundleOwnsComposedRuntimeGraphAndProvenance(t *testing.T) {
	dir := t.TempDir()
	writeSourceGraphScript(t, dir, "base.lua", `local c=require("cervterm"); return {font={family="Base"},shell={args={"pwsh"},env={A="one"}},keys={{key="k",action=c.action.ScrollPage(1)}},events={output=function() end}}`)
	primary := writeSourceGraphScript(t, dir, "primary.lua", `local c=require("cervterm"); c.after(10,function() end); return {config_version=2,includes={"base.lua"},default_profile="work",profiles={work={font={size=16},events={title=function() end}}}}`)
	bundle, err := BuildCandidateBundle(primary, config.Defaults(), CandidateOptions{Composition: config.CompositionOptions{CLIOverrides: []config.CLIOverride{{ArgumentIndex: 2, Path: "font.family", Value: "CLI Font"}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	if bundle.Config().Font.Family != "CLI Font" || bundle.Config().Font.Size != 16 {
		t.Fatalf("bundle config = %#v", bundle.Config().Font)
	}
	copy := bundle.Config()
	copy.Shell.Args[0] = "mutated"
	copy.Shell.Env["A"] = "mutated"
	if fresh := bundle.Config(); fresh.Shell.Args[0] != "pwsh" || fresh.Shell.Env["A"] != "one" {
		t.Fatalf("config accessor leaked mutable bundle state: %#v", fresh.Shell)
	}
	runtime := bundle.runtime
	if runtime == nil || len(runtime.Bindings()) != 1 || !runtime.WantsOutput() || runtime.events.title == nil || len(runtime.timers.entries) != 1 {
		t.Fatalf("bundle runtime bindings=%#v events=%#v timers=%#v", runtime.Bindings(), runtime.events, runtime.timers.entries)
	}
	selection := bundle.Selection()
	if selection.Profile == nil || selection.Profile.Name != "work" {
		t.Fatalf("selection = %#v", selection)
	}
	selection.Profile.Name = "mutated"
	if bundle.Selection().Profile.Name != "work" {
		t.Fatal("selection accessor leaked mutable bundle state")
	}
	var family config.ProvenanceRecord
	for _, record := range bundle.Provenance() {
		if record.Path == "font.family" {
			family = record
		}
	}
	if family.Winner.Layer != config.LayerCLI || !family.Winner.HasCLIArgumentIndex || family.Winner.CLIArgumentIndex != 2 {
		t.Fatalf("font.family provenance = %#v", family)
	}
	if _, _, err := Load(primary, config.Defaults()); err == nil || !strings.Contains(err.Error(), "not available yet") {
		t.Fatalf("public loader unexpectedly activated composition: %v", err)
	}
}

func TestCandidateBundleRejectsMalformedOverwrittenV1ScriptingSurface(t *testing.T) {
	dir := t.TempDir()
	writeSourceGraphScript(t, dir, "legacy.lua", `return {keys={{key=3,action=function() end}}}`)
	primary := writeSourceGraphScript(t, dir, "primary.lua", `return {config_version=2,includes={"legacy.lua"},keys={{key="a",action=function() end}}}`)
	bundle, err := BuildCandidateBundle(primary, config.Defaults(), CandidateOptions{})
	if bundle != nil {
		bundle.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "legacy.lua") || !strings.Contains(err.Error(), "keys[1]: key must be a string") {
		t.Fatalf("per-source legacy scripting error = %v", err)
	}
}

func TestCandidateBundleCloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	primary := writeSourceGraphScript(t, dir, "primary.lua", `return {config_version=2}`)
	base := config.Defaults()
	base.Shell.Args = []string{"base"}
	base.Shell.Env = map[string]string{"A": "base"}
	bundle, err := BuildCandidateBundle(primary, base, CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.runtime == nil {
		t.Fatal("candidate runtime missing")
	}
	base.Shell.Args[0] = "mutated"
	base.Shell.Env["A"] = "mutated"
	if got := bundle.Config(); got.Shell.Args[0] != "base" || got.Shell.Env["A"] != "base" {
		bundle.Close()
		t.Fatalf("caller base mutation changed candidate: %#v", got.Shell)
	}
	activation, err := bundle.PrepareActivation()
	if err != nil || activation.Commit() != bundle.runtime {
		bundle.Close()
		t.Fatalf("candidate activation runtime=%p err=%v", activation.Commit(), err)
	}
	bundle.Close()
	bundle.Close()
	if bundle.runtime != nil {
		t.Fatal("closed bundle exposed runtime")
	}
	if activation.Commit() != nil {
		t.Fatal("closed bundle left activation handle usable")
	}
	if _, err := bundle.PrepareActivation(); err == nil {
		t.Fatal("closed bundle prepared activation")
	}
	if _, err := bundle.PublishTeal(config.TealPublicationOptions{}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("closed publication error = %v", err)
	}
}

func TestCandidateBundleBuildFailureCleansOwnedStaging(t *testing.T) {
	dir := t.TempDir()
	stagingParent := filepath.Join(dir, "staging")
	primary := writeSourceGraphScript(t, dir, "primary.lua", `return {config_version=2,font={size=-1}}`)
	bundle, err := BuildCandidateBundle(primary, config.Defaults(), CandidateOptions{SourceGraph: config.SourceGraphOptions{StageDirectory: stagingParent}})
	if bundle != nil {
		bundle.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "font size") {
		t.Fatalf("invalid candidate error = %v", err)
	}
	entries, readErr := os.ReadDir(stagingParent)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed candidate staging remains: %#v", entries)
	}
}

func TestCandidateBundleDefersAndOwnsTealPublication(t *testing.T) {
	if _, err := exec.LookPath("tl"); err != nil {
		t.Skip("tl not installed")
	}
	dir := t.TempDir()
	copyFile(t, filepath.Join("..", "..", "docs", "examples", "cervterm.d.tl"), filepath.Join(dir, "cervterm.d.tl"))
	source := filepath.Join(dir, "config.tl")
	body := `local c=require("cervterm")
local cfg: c.Config = {config_version=2,font={family="Mono",size=14,ligatures=true}}
return cfg
`
	if err := os.WriteFile(source, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildCandidateBundle(source, config.Defaults(), CandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	published := filepath.Join(dir, "config.lua")
	if _, err := os.Stat(published); !os.IsNotExist(err) {
		bundle.Close()
		t.Fatalf("bundle build published Teal early: %v", err)
	}
	staged := bundle.graph.StagedTeal[0].EvaluationLua
	if err := os.WriteFile(published, []byte("foreign"), 0o600); err != nil {
		bundle.Close()
		t.Fatal(err)
	}
	if _, err := bundle.PublishTeal(config.TealPublicationOptions{}); err == nil || !strings.Contains(err.Error(), "unowned") {
		bundle.Close()
		t.Fatalf("publication failure = %v", err)
	}
	if bundle.runtime == nil {
		bundle.Close()
		t.Fatal("publication failure released candidate runtime")
	}
	if err := os.Remove(published); err != nil {
		bundle.Close()
		t.Fatal(err)
	}
	result, err := bundle.PublishTeal(config.TealPublicationOptions{})
	if err != nil || len(result.Outputs) != 1 {
		bundle.Close()
		t.Fatalf("bundle publication=%#v err=%v", result, err)
	}
	result.Outputs[0].SourcePath = "mutated"
	second, err := bundle.PublishTeal(config.TealPublicationOptions{})
	if err != nil || len(second.Outputs) != 1 {
		bundle.Close()
		t.Fatalf("idempotent publication=%#v err=%v", second, err)
	}
	if second.Outputs[0].SourcePath == "mutated" {
		bundle.Close()
		t.Fatal("publication result accessor leaked mutable bundle state")
	}
	bundle.Close()
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("candidate staging remains after close: %v", err)
	}
	if _, err := os.Stat(published); err != nil {
		t.Fatalf("published compatibility output missing: %v", err)
	}
}
